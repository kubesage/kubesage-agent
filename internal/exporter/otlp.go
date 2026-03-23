package exporter

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/grpc/credentials"
)

// NewMeterProvider creates an OTLP/gRPC meter provider with optional mTLS and gzip compression.
// If tlsCreds is nil, the exporter uses an insecure connection (for development).
func NewMeterProvider(ctx context.Context, endpoint string, tlsCreds credentials.TransportCredentials, res *resource.Resource, interval time.Duration) (*metric.MeterProvider, error) {
	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(endpoint),
		otlpmetricgrpc.WithCompressor("gzip"),
	}

	if tlsCreds != nil {
		opts = append(opts, otlpmetricgrpc.WithTLSCredentials(tlsCreds))
	} else {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}

	exp, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP exporter: %w", err)
	}

	mp := metric.NewMeterProvider(
		metric.WithReader(
			metric.NewPeriodicReader(exp,
				metric.WithInterval(interval),
			),
		),
		metric.WithResource(res),
	)

	return mp, nil
}

// LoadTLSCredentials loads mTLS client certificates and CA certificate for gRPC transport.
func LoadTLSCredentials(certFile, keyFile, caFile string) (credentials.TransportCredentials, error) {
	// Load client certificate and key
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("loading client certificate: %w", err)
	}

	// Load CA certificate
	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("reading CA certificate: %w", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to add CA certificate to pool")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      certPool,
	}

	return credentials.NewTLS(tlsConfig), nil
}
