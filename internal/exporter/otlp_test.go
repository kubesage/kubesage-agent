package exporter

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMeterProvider_Insecure(t *testing.T) {
	ctx := context.Background()
	res := resource.Default()

	mp, err := NewMeterProvider(ctx, "localhost:4317", nil, res, 30*time.Second)
	require.NoError(t, err)
	require.NotNil(t, mp)

	// Verify we can create a meter from the provider
	meter := mp.Meter("test")
	assert.NotNil(t, meter)

	// Shutdown may return a connection error since no collector is running,
	// but the meter provider itself was created successfully, which is what
	// we are validating. We use a short timeout to avoid long waits.
	shutdownCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_ = mp.Shutdown(shutdownCtx) // best-effort; error expected without a real collector
}

func TestLoadTLSCredentials_NonExistentFiles(t *testing.T) {
	_, err := LoadTLSCredentials("/nonexistent/cert.pem", "/nonexistent/key.pem", "/nonexistent/ca.pem")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading client certificate")
}
