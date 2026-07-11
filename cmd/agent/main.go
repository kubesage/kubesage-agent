package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/kubesage/kubesage-agent/internal/api"
	"github.com/kubesage/kubesage-agent/internal/collector"
	"github.com/kubesage/kubesage-agent/internal/config"
	"github.com/kubesage/kubesage-agent/internal/exporter"
	"github.com/kubesage/kubesage-agent/internal/health"
	"github.com/kubesage/kubesage-agent/internal/metrics"
)

// version is the agent version reported in heartbeats. Override at build time
// via -ldflags "-X main.version=<tag>".
var version = "dev"

func main() {
	cfg, err := config.Parse()
	if err != nil {
		// Use a basic logger for startup errors since zap isn't configured yet
		panic("failed to parse config: " + err.Error())
	}

	// Create zap logger based on configured log level
	logger := newLogger(cfg.LogLevel)
	defer logger.Sync()

	// Set OTel error handler to surface hidden SDK errors
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.Error("OTel SDK error", zap.Error(err))
	}))

	// Create OTel resource with cluster + tenant attributes (cluster_id becomes a metric label)
	res := metrics.NewResource(cfg.ClusterName, cfg.TenantID, cfg.ClusterID)

	// Set up context with signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Load TLS credentials if cert files exist, otherwise use insecure mode
	var tlsCreds interface{ String() string } // credentials.TransportCredentials
	certFile := filepath.Join(cfg.CertDir, "tls.crt")
	keyFile := filepath.Join(cfg.CertDir, "tls.key")
	caFile := filepath.Join(cfg.CertDir, "ca.crt")

	grpcCreds, tlsErr := exporter.LoadTLSCredentials(certFile, keyFile, caFile)
	if tlsErr != nil {
		logger.Warn("TLS certificates not found, using insecure connection", zap.Error(tlsErr))
		grpcCreds = nil
	} else {
		logger.Info("Loaded TLS credentials for mTLS")
	}
	_ = tlsCreds // unused placeholder removed in favor of grpcCreds

	// Create OTLP/gRPC meter provider
	mp, err := exporter.NewMeterProvider(ctx, cfg.Endpoint, grpcCreds, res, cfg.ScrapeInterval)
	if err != nil {
		logger.Fatal("Failed to create meter provider", zap.Error(err))
	}

	// Create OTel metric instruments
	meter := mp.Meter("kubesage-agent")
	instruments, err := metrics.NewInstruments(meter)
	if err != nil {
		logger.Fatal("Failed to create metric instruments", zap.Error(err))
	}

	// Create Kubernetes clientset (in-cluster or kubeconfig for dev)
	k8sClient, err := newKubernetesClient(logger)
	if err != nil {
		logger.Fatal("Failed to create Kubernetes client", zap.Error(err))
	}

	// Create and start the metric collector
	coll := collector.New(k8sClient, instruments, logger, cfg.ClusterName, cfg.ScrapeInterval)
	collectorErrCh := make(chan error, 1)
	go func() {
		if err := coll.Start(ctx); err != nil {
			collectorErrCh <- err
		}
	}()

	// Construct the REST API client and drive a tenant-scoped heartbeat loop.
	// Mirrors the collector goroutine shape: non-blocking, ctx-cancelled, errors
	// logged (never fatal — a transient heartbeat failure must not crash the agent).
	apiClient := api.NewClient(cfg.APIURL, cfg.Token, cfg.TenantID, cfg.ClusterID)
	go func() {
		ticker := time.NewTicker(cfg.ScrapeInterval)
		defer ticker.Stop()
		// Emit an initial heartbeat immediately so the cluster flips to connected.
		sendHeartbeat(ctx, apiClient, k8sClient, logger)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sendHeartbeat(ctx, apiClient, k8sClient, logger)
			}
		}
	}()

	// Start health check server
	healthServer := health.New(cfg.HealthPort)
	go func() {
		if err := healthServer.Start(ctx); err != nil {
			logger.Error("Health server error", zap.Error(err))
		}
	}()

	// Mark as ready after initialization
	healthServer.SetReady()

	logger.Info("KubeSage agent started",
		zap.String("endpoint", cfg.Endpoint),
		zap.String("cluster", cfg.ClusterName),
		zap.Duration("scrape_interval", cfg.ScrapeInterval),
		zap.Int("health_port", cfg.HealthPort),
	)

	// Wait for shutdown signal or collector error
	select {
	case sig := <-sigCh:
		logger.Info("Received shutdown signal", zap.String("signal", sig.String()))
	case err := <-collectorErrCh:
		logger.Error("Collector error, shutting down", zap.Error(err))
	}
	cancel()

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := mp.Shutdown(shutdownCtx); err != nil {
		logger.Error("Meter provider shutdown error", zap.Error(err))
	}

	logger.Info("KubeSage agent stopped")
}

// sendHeartbeat collects the current node/pod counts and reports a heartbeat to
// the tenant-scoped API. Errors are logged, never fatal. The client never logs
// the token (T-39-03-02), so a failed heartbeat cannot leak the credential.
func sendHeartbeat(ctx context.Context, client *api.Client, k8sClient kubernetes.Interface, logger *zap.Logger) {
	nodeCount, podCount := clusterCounts(ctx, k8sClient, logger)
	if err := client.Heartbeat(ctx, version, nodeCount, podCount); err != nil {
		logger.Warn("Heartbeat failed", zap.Error(err))
	}
}

// clusterCounts returns the current node and pod counts from the K8s API.
// On error it degrades gracefully (logs at debug, returns best-effort counts).
func clusterCounts(ctx context.Context, k8sClient kubernetes.Interface, logger *zap.Logger) (int, int) {
	nodes, err := k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Debug("Failed to list nodes for heartbeat", zap.Error(err))
		return 0, 0
	}
	pods, err := k8sClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Debug("Failed to list pods for heartbeat", zap.Error(err))
		return len(nodes.Items), 0
	}
	return len(nodes.Items), len(pods.Items)
}

// newKubernetesClient creates a Kubernetes clientset using in-cluster config
// with a fallback to KUBECONFIG environment variable for local development.
func newKubernetesClient(logger *zap.Logger) (kubernetes.Interface, error) {
	// Try in-cluster config first (running inside K8s)
	restCfg, err := rest.InClusterConfig()
	if err != nil {
		logger.Info("Not running in-cluster, falling back to kubeconfig")
		// Fallback to kubeconfig for local development
		kubeconfigPath := os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" {
			home, _ := os.UserHomeDir()
			kubeconfigPath = filepath.Join(home, ".kube", "config")
		}
		restCfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, err
		}
	}

	return kubernetes.NewForConfig(restCfg)
}

func newLogger(level string) *zap.Logger {
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	zapCfg := zap.NewProductionConfig()
	zapCfg.Level = zap.NewAtomicLevelAt(zapLevel)
	logger, _ := zapCfg.Build()
	return logger
}
