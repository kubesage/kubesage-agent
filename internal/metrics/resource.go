package metrics

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// NewResource creates an OTel resource with cluster and tenant attributes.
func NewResource(clusterName, tenantID string) *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		attribute.String("k8s.cluster.name", clusterName),
		attribute.String("kubesage.tenant.id", tenantID),
	)
}
