package config

import (
	"fmt"
	"time"

	"github.com/spf13/pflag"
)

// Config holds the agent configuration parsed from flags and environment variables.
type Config struct {
	Endpoint       string
	APIURL         string
	Token          string
	ClusterName    string
	ScrapeInterval time.Duration
	LogLevel       string
	HealthPort     int
	CertDir        string
}

// Parse parses agent configuration from command-line flags and environment variables.
// Flags take precedence over environment variables. Endpoint and Token are required.
func Parse() (*Config, error) {
	cfg := &Config{}

	pflag.StringVar(&cfg.Endpoint, "endpoint", "", "Platform gRPC endpoint (env: KUBESAGE_ENDPOINT)")
	pflag.StringVar(&cfg.APIURL, "api-url", "", "REST API base URL (env: KUBESAGE_API_URL, default: http://localhost:8080)")
	pflag.StringVar(&cfg.Token, "token", "", "Cluster bootstrap token (env: KUBESAGE_TOKEN)")
	pflag.StringVar(&cfg.ClusterName, "cluster-name", "", "Cluster name (env: KUBESAGE_CLUSTER_NAME, default: auto-detect)")
	pflag.DurationVar(&cfg.ScrapeInterval, "scrape-interval", 30*time.Second, "Metrics scrape interval")
	pflag.StringVar(&cfg.LogLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	pflag.IntVar(&cfg.HealthPort, "health-port", 8080, "Health check HTTP port")
	pflag.StringVar(&cfg.CertDir, "cert-dir", "/etc/kubesage/certs", "Directory containing TLS certificates")

	pflag.Parse()

	// Environment variable fallbacks
	if cfg.Endpoint == "" {
		cfg.Endpoint = envOrDefault("KUBESAGE_ENDPOINT", "")
	}
	if cfg.Token == "" {
		cfg.Token = envOrDefault("KUBESAGE_TOKEN", "")
	}
	if cfg.APIURL == "" {
		cfg.APIURL = envOrDefault("KUBESAGE_API_URL", "http://localhost:8080")
	}
	if cfg.ClusterName == "" {
		cfg.ClusterName = envOrDefault("KUBESAGE_CLUSTER_NAME", "")
	}

	// Validate required fields
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("--endpoint or KUBESAGE_ENDPOINT is required")
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("--token or KUBESAGE_TOKEN is required")
	}

	return cfg, nil
}
