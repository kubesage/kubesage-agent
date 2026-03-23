package config

import (
	"os"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetFlags() {
	pflag.CommandLine = pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
}

func TestParse_MissingEndpoint(t *testing.T) {
	resetFlags()
	os.Args = []string{"agent", "--token=test-token"}
	os.Unsetenv("KUBESAGE_ENDPOINT")

	_, err := Parse()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "endpoint")
}

func TestParse_MissingToken(t *testing.T) {
	resetFlags()
	os.Args = []string{"agent", "--endpoint=localhost:4317"}
	os.Unsetenv("KUBESAGE_TOKEN")

	_, err := Parse()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token")
}

func TestParse_Defaults(t *testing.T) {
	resetFlags()
	os.Args = []string{"agent", "--endpoint=localhost:4317", "--token=test-token"}

	cfg, err := Parse()
	require.NoError(t, err)
	assert.Equal(t, "localhost:4317", cfg.Endpoint)
	assert.Equal(t, "test-token", cfg.Token)
	assert.Equal(t, 30*time.Second, cfg.ScrapeInterval)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, 8080, cfg.HealthPort)
	assert.Equal(t, "/etc/kubesage/certs", cfg.CertDir)
}

func TestParse_EnvFallback(t *testing.T) {
	resetFlags()
	os.Args = []string{"agent"}
	os.Setenv("KUBESAGE_ENDPOINT", "env-endpoint:4317")
	os.Setenv("KUBESAGE_TOKEN", "env-token")
	defer os.Unsetenv("KUBESAGE_ENDPOINT")
	defer os.Unsetenv("KUBESAGE_TOKEN")

	cfg, err := Parse()
	require.NoError(t, err)
	assert.Equal(t, "env-endpoint:4317", cfg.Endpoint)
	assert.Equal(t, "env-token", cfg.Token)
}
