package metrics

import (
	"go.opentelemetry.io/otel/metric"
)

// Instruments holds all OTel metric instruments for K8s cluster monitoring.
type Instruments struct {
	// Pod CPU metrics
	PodCPUUsage   metric.Float64Gauge
	PodCPURequest metric.Float64Gauge
	PodCPULimit   metric.Float64Gauge

	// Pod memory metrics
	PodMemoryUsage   metric.Int64Gauge
	PodMemoryRequest metric.Int64Gauge
	PodMemoryLimit   metric.Int64Gauge

	// Node CPU metrics
	NodeCPUUsage       metric.Float64Gauge
	NodeCPUAllocatable metric.Float64Gauge

	// Node memory metrics
	NodeMemoryUsage       metric.Int64Gauge
	NodeMemoryAllocatable metric.Int64Gauge

	// Pod status
	PodPhase metric.Int64Gauge

	// Container health
	ContainerRestarts metric.Int64Gauge

	// Namespace counts
	NamespacePodCount metric.Int64Gauge

	// Node conditions
	NodeCondition metric.Int64Gauge

	// Deployment status
	DeploymentAvailable metric.Int64Gauge
	DeploymentDesired   metric.Int64Gauge
}

// NewInstruments creates all 17 OTel metric instruments for K8s monitoring.
// Attributes used across metrics: k8s.cluster.name, k8s.namespace.name,
// k8s.node.name, k8s.pod.name, k8s.deployment.name (where applicable).
func NewInstruments(meter metric.Meter) (*Instruments, error) {
	var inst Instruments
	var err error

	// Pod CPU
	inst.PodCPUUsage, err = meter.Float64Gauge("k8s.pod.cpu.usage",
		metric.WithUnit("cores"),
		metric.WithDescription("CPU usage of the pod in cores"),
	)
	if err != nil {
		return nil, err
	}

	inst.PodCPURequest, err = meter.Float64Gauge("k8s.pod.cpu.request",
		metric.WithUnit("cores"),
		metric.WithDescription("CPU request of the pod in cores"),
	)
	if err != nil {
		return nil, err
	}

	inst.PodCPULimit, err = meter.Float64Gauge("k8s.pod.cpu.limit",
		metric.WithUnit("cores"),
		metric.WithDescription("CPU limit of the pod in cores"),
	)
	if err != nil {
		return nil, err
	}

	// Pod Memory
	inst.PodMemoryUsage, err = meter.Int64Gauge("k8s.pod.memory.usage",
		metric.WithUnit("By"),
		metric.WithDescription("Memory usage of the pod in bytes"),
	)
	if err != nil {
		return nil, err
	}

	inst.PodMemoryRequest, err = meter.Int64Gauge("k8s.pod.memory.request",
		metric.WithUnit("By"),
		metric.WithDescription("Memory request of the pod in bytes"),
	)
	if err != nil {
		return nil, err
	}

	inst.PodMemoryLimit, err = meter.Int64Gauge("k8s.pod.memory.limit",
		metric.WithUnit("By"),
		metric.WithDescription("Memory limit of the pod in bytes"),
	)
	if err != nil {
		return nil, err
	}

	// Node CPU
	inst.NodeCPUUsage, err = meter.Float64Gauge("k8s.node.cpu.usage",
		metric.WithUnit("cores"),
		metric.WithDescription("CPU usage of the node in cores"),
	)
	if err != nil {
		return nil, err
	}

	inst.NodeCPUAllocatable, err = meter.Float64Gauge("k8s.node.cpu.allocatable",
		metric.WithUnit("cores"),
		metric.WithDescription("Allocatable CPU of the node in cores"),
	)
	if err != nil {
		return nil, err
	}

	// Node Memory
	inst.NodeMemoryUsage, err = meter.Int64Gauge("k8s.node.memory.usage",
		metric.WithUnit("By"),
		metric.WithDescription("Memory usage of the node in bytes"),
	)
	if err != nil {
		return nil, err
	}

	inst.NodeMemoryAllocatable, err = meter.Int64Gauge("k8s.node.memory.allocatable",
		metric.WithUnit("By"),
		metric.WithDescription("Allocatable memory of the node in bytes"),
	)
	if err != nil {
		return nil, err
	}

	// Pod Phase (1 per phase per namespace)
	inst.PodPhase, err = meter.Int64Gauge("k8s.pod.phase",
		metric.WithDescription("Number of pods in each phase per namespace"),
	)
	if err != nil {
		return nil, err
	}

	// Container Restarts
	inst.ContainerRestarts, err = meter.Int64Gauge("k8s.container.restarts",
		metric.WithDescription("Total number of container restarts"),
	)
	if err != nil {
		return nil, err
	}

	// Namespace Pod Count
	inst.NamespacePodCount, err = meter.Int64Gauge("k8s.namespace.pod.count",
		metric.WithDescription("Number of pods in a namespace"),
	)
	if err != nil {
		return nil, err
	}

	// Node Condition (1/0 per condition type)
	inst.NodeCondition, err = meter.Int64Gauge("k8s.node.condition",
		metric.WithDescription("Node condition status (1=true, 0=false)"),
	)
	if err != nil {
		return nil, err
	}

	// Deployment Status
	inst.DeploymentAvailable, err = meter.Int64Gauge("k8s.deployment.available",
		metric.WithDescription("Number of available replicas in a deployment"),
	)
	if err != nil {
		return nil, err
	}

	inst.DeploymentDesired, err = meter.Int64Gauge("k8s.deployment.desired",
		metric.WithDescription("Number of desired replicas in a deployment"),
	)
	if err != nil {
		return nil, err
	}

	return &inst, nil
}
