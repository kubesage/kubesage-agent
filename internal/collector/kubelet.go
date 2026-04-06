package collector

import (
	"context"
	"encoding/json"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"

	"github.com/kubesage/kubesage-agent/internal/metrics"
)

// Kubelet stats/summary API response types (only the fields we need).

// Summary is the top-level kubelet stats/summary response.
type Summary struct {
	Node NodeStats  `json:"node"`
	Pods []PodStats `json:"pods"`
}

// NodeStats holds node-level CPU and memory usage.
type NodeStats struct {
	CPU    *CPUStats    `json:"cpu"`
	Memory *MemoryStats `json:"memory"`
}

// PodStats holds per-pod CPU and memory usage.
type PodStats struct {
	PodRef PodRef       `json:"podRef"`
	CPU    *CPUStats    `json:"cpu"`
	Memory *MemoryStats `json:"memory"`
}

// PodRef identifies a pod by name and namespace.
type PodRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// CPUStats holds CPU usage in nanocores.
type CPUStats struct {
	UsageNanoCores *uint64 `json:"usageNanoCores"`
}

// MemoryStats holds memory usage in bytes.
type MemoryStats struct {
	UsageBytes *uint64 `json:"usageBytes"`
}

// KubeletCollector scrapes the kubelet stats/summary API for real-time CPU and memory
// usage metrics per node and pod.
type KubeletCollector struct {
	clientset   kubernetes.Interface
	inst        *metrics.Instruments
	logger      *zap.Logger
	clusterName string
	maxWorkers  int
}

// NewKubeletCollector creates a KubeletCollector with a default worker pool size of 10.
func NewKubeletCollector(clientset kubernetes.Interface, inst *metrics.Instruments, logger *zap.Logger, clusterName string) *KubeletCollector {
	return &KubeletCollector{
		clientset:   clientset,
		inst:        inst,
		logger:      logger,
		clusterName: clusterName,
		maxWorkers:  10,
	}
}

// Collect scrapes kubelet stats/summary for each node in parallel using a worker pool.
// Errors on individual nodes are logged but do not fail the entire collection.
func (kc *KubeletCollector) Collect(ctx context.Context, nodeNames []string) {
	sem := make(chan struct{}, kc.maxWorkers)
	var wg sync.WaitGroup

	for _, nodeName := range nodeNames {
		wg.Add(1)
		sem <- struct{}{} // acquire semaphore slot
		go func(name string) {
			defer wg.Done()
			defer func() { <-sem }() // release semaphore slot

			kc.scrapeNode(ctx, name)
		}(nodeName)
	}

	wg.Wait()
}

func (kc *KubeletCollector) scrapeNode(ctx context.Context, nodeName string) {
	data, err := kc.clientset.CoreV1().RESTClient().
		Get().
		Resource("nodes").
		Name(nodeName).
		SubResource("proxy", "stats", "summary").
		DoRaw(ctx)
	if err != nil {
		kc.logger.Warn("Failed to scrape kubelet stats",
			zap.String("node", nodeName),
			zap.Error(err),
		)
		return
	}

	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		kc.logger.Warn("Failed to parse kubelet stats",
			zap.String("node", nodeName),
			zap.Error(err),
		)
		return
	}

	nodeAttrs := otelmetric.WithAttributeSet(attribute.NewSet(
		attribute.String("k8s.cluster.name", kc.clusterName),
		attribute.String("k8s.node.name", nodeName),
	))

	// Node CPU usage (convert nanocores to cores)
	if summary.Node.CPU != nil && summary.Node.CPU.UsageNanoCores != nil {
		cpuCores := float64(*summary.Node.CPU.UsageNanoCores) / 1e9
		kc.inst.NodeCPUUsage.Record(ctx, cpuCores, nodeAttrs)
	}

	// Node memory usage
	if summary.Node.Memory != nil && summary.Node.Memory.UsageBytes != nil {
		kc.inst.NodeMemoryUsage.Record(ctx, int64(*summary.Node.Memory.UsageBytes), nodeAttrs)
	}

	// Per-pod usage metrics
	for _, pod := range summary.Pods {
		podAttrs := otelmetric.WithAttributeSet(attribute.NewSet(
			attribute.String("k8s.cluster.name", kc.clusterName),
			attribute.String("k8s.namespace.name", pod.PodRef.Namespace),
			attribute.String("k8s.pod.name", pod.PodRef.Name),
			attribute.String("k8s.node.name", nodeName),
		))

		if pod.CPU != nil && pod.CPU.UsageNanoCores != nil {
			cpuCores := float64(*pod.CPU.UsageNanoCores) / 1e9
			kc.inst.PodCPUUsage.Record(ctx, cpuCores, podAttrs)
		}

		if pod.Memory != nil && pod.Memory.UsageBytes != nil {
			kc.inst.PodMemoryUsage.Record(ctx, int64(*pod.Memory.UsageBytes), podAttrs)
		}
	}
}
