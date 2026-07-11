package metrics

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// NewResource creates an OTel resource with cluster and tenant attributes.
// The cluster_id attribute becomes a metric label so VM series can be filtered
// per registered cluster (tenant scoping stays via the vmauth token→tenant_id).
func NewResource(clusterName, tenantID, clusterID string) *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		attribute.String("k8s.cluster.name", clusterName),
		attribute.String("kubesage.tenant.id", tenantID),
		attribute.String("cluster_id", clusterID),
	)
}
