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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/kubesage/kubesage-agent/internal/collector"
	"github.com/kubesage/kubesage-agent/internal/config"
	"github.com/kubesage/kubesage-agent/internal/exporter"
	"github.com/kubesage/kubesage-agent/internal/health"
	"github.com/kubesage/kubesage-agent/internal/metrics"
)

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

	// Create OTel resource with cluster attributes
	res := metrics.NewResource(cfg.ClusterName, "")

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
