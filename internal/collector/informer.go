package collector

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"

	"github.com/kubesage/kubesage-agent/internal/metrics"
)

// InformerCollector watches K8s API objects via SharedInformerFactory and records
// resource spec metrics (requests, limits, pod counts, node conditions, deployment status).
type InformerCollector struct {
	factory     informers.SharedInformerFactory
	inst        *metrics.Instruments
	logger      *zap.Logger
	clusterName string

	podLister        corelisters.PodLister
	nodeLister       corelisters.NodeLister
	deploymentLister appslisters.DeploymentLister
}

// NewInformerCollector creates an InformerCollector with transform functions that strip
// unnecessary fields to stay within the 300MB memory budget.
func NewInformerCollector(clientset kubernetes.Interface, inst *metrics.Instruments, logger *zap.Logger, clusterName string, resyncPeriod time.Duration) *InformerCollector {
	factory := informers.NewSharedInformerFactoryWithOptions(clientset, resyncPeriod,
		informers.WithTransform(transformObject),
	)

	ic := &InformerCollector{
		factory:     factory,
		inst:        inst,
		logger:      logger,
		clusterName: clusterName,
	}

	// Register informers and grab listers
	ic.podLister = factory.Core().V1().Pods().Lister()
	ic.nodeLister = factory.Core().V1().Nodes().Lister()
	ic.deploymentLister = factory.Apps().V1().Deployments().Lister()

	return ic
}

// Start begins watching K8s API objects and blocks until the cache is synced or ctx is cancelled.
func (ic *InformerCollector) Start(ctx context.Context) error {
	ic.factory.Start(ctx.Done())

	synced := ic.factory.WaitForCacheSync(ctx.Done())
	for typ, ok := range synced {
		if !ok {
			return fmt.Errorf("informer cache sync failed for %v", typ)
		}
	}

	ic.logger.Info("Informer cache synced",
		zap.Int("types", len(synced)),
	)
	return nil
}

// NodeNames returns the list of node names from the informer cache.
func (ic *InformerCollector) NodeNames() []string {
	nodes, err := ic.nodeLister.List(labels.Everything())
	if err != nil {
		ic.logger.Error("Failed to list nodes", zap.Error(err))
		return nil
	}
	names := make([]string, 0, len(nodes))
	for _, n := range nodes {
		names = append(names, n.Name)
	}
	return names
}

// Collect reads cached K8s objects and records metrics for pods, nodes, and deployments.
func (ic *InformerCollector) Collect(ctx context.Context) {
	ic.collectPods(ctx)
	ic.collectNodes(ctx)
	ic.collectDeployments(ctx)
}

func (ic *InformerCollector) collectPods(ctx context.Context) {
	pods, err := ic.podLister.List(labels.Everything())
	if err != nil {
		ic.logger.Error("Failed to list pods", zap.Error(err))
		return
	}

	// Aggregate pod counts per namespace
	nsPodCount := make(map[string]int64)

	for _, pod := range pods {
		ns := pod.Namespace
		name := pod.Name
		nsPodCount[ns]++

		baseAttrs := otelmetric.WithAttributeSet(attribute.NewSet(
			attribute.String("k8s.cluster.name", ic.clusterName),
			attribute.String("k8s.namespace.name", ns),
			attribute.String("k8s.pod.name", name),
		))

		// Pod phase
		phase := string(pod.Status.Phase)
		ic.inst.PodPhase.Record(ctx, 1,
			otelmetric.WithAttributeSet(attribute.NewSet(
				attribute.String("k8s.cluster.name", ic.clusterName),
				attribute.String("k8s.namespace.name", ns),
				attribute.String("k8s.pod.name", name),
				attribute.String("k8s.pod.phase", phase),
			)),
		)

		// Container restarts
		var totalRestarts int64
		for _, cs := range pod.Status.ContainerStatuses {
			totalRestarts += int64(cs.RestartCount)
		}
		ic.inst.ContainerRestarts.Record(ctx, totalRestarts, baseAttrs)

		// Sum resource requests/limits across containers
		var cpuReq, cpuLim float64
		var memReq, memLim int64
		for _, c := range pod.Spec.Containers {
			cpuReq += quantityToFloat64(c.Resources.Requests.Cpu())
			cpuLim += quantityToFloat64(c.Resources.Limits.Cpu())
			memReq += c.Resources.Requests.Memory().Value()
			memLim += c.Resources.Limits.Memory().Value()
		}
		ic.inst.PodCPURequest.Record(ctx, cpuReq, baseAttrs)
		ic.inst.PodCPULimit.Record(ctx, cpuLim, baseAttrs)
		ic.inst.PodMemoryRequest.Record(ctx, memReq, baseAttrs)
		ic.inst.PodMemoryLimit.Record(ctx, memLim, baseAttrs)
	}

	// k8s.namespace.pod.count per namespace
	for ns, count := range nsPodCount {
		ic.inst.NamespacePodCount.Record(ctx, count,
			otelmetric.WithAttributeSet(attribute.NewSet(
				attribute.String("k8s.cluster.name", ic.clusterName),
				attribute.String("k8s.namespace.name", ns),
			)),
		)
	}
}

func (ic *InformerCollector) collectNodes(ctx context.Context) {
	nodes, err := ic.nodeLister.List(labels.Everything())
	if err != nil {
		ic.logger.Error("Failed to list nodes", zap.Error(err))
		return
	}

	for _, node := range nodes {
		baseAttrs := otelmetric.WithAttributeSet(attribute.NewSet(
			attribute.String("k8s.cluster.name", ic.clusterName),
			attribute.String("k8s.node.name", node.Name),
		))

		// Node allocatable
		allocCPU := quantityToFloat64(node.Status.Allocatable.Cpu())
		allocMem := node.Status.Allocatable.Memory().Value()
		ic.inst.NodeCPUAllocatable.Record(ctx, allocCPU, baseAttrs)
		ic.inst.NodeMemoryAllocatable.Record(ctx, allocMem, baseAttrs)

		// Node conditions: Ready, MemoryPressure, DiskPressure
		for _, cond := range node.Status.Conditions {
			switch cond.Type {
			case corev1.NodeReady, corev1.NodeMemoryPressure, corev1.NodeDiskPressure:
				var val int64
				if cond.Status == corev1.ConditionTrue {
					val = 1
				}
				ic.inst.NodeCondition.Record(ctx, val,
					otelmetric.WithAttributeSet(attribute.NewSet(
						attribute.String("k8s.cluster.name", ic.clusterName),
						attribute.String("k8s.node.name", node.Name),
						attribute.String("k8s.node.condition", string(cond.Type)),
					)),
				)
			}
		}
	}
}

func (ic *InformerCollector) collectDeployments(ctx context.Context) {
	deployments, err := ic.deploymentLister.List(labels.Everything())
	if err != nil {
		ic.logger.Error("Failed to list deployments", zap.Error(err))
		return
	}

	for _, dep := range deployments {
		attrs := otelmetric.WithAttributeSet(attribute.NewSet(
			attribute.String("k8s.cluster.name", ic.clusterName),
			attribute.String("k8s.namespace.name", dep.Namespace),
			attribute.String("k8s.deployment.name", dep.Name),
		))

		ic.inst.DeploymentAvailable.Record(ctx, int64(dep.Status.AvailableReplicas), attrs)
		var desired int64
		if dep.Spec.Replicas != nil {
			desired = int64(*dep.Spec.Replicas)
		}
		ic.inst.DeploymentDesired.Record(ctx, desired, attrs)
	}
}

// transformObject strips unnecessary fields from K8s objects before caching.
// This is CRITICAL for staying within the 300MB memory budget.
func transformObject(obj interface{}) (interface{}, error) {
	switch o := obj.(type) {
	case *corev1.Pod:
		return transformPod(o), nil
	case *corev1.Node:
		return transformNode(o), nil
	case *appsv1.Deployment:
		return transformDeployment(o), nil
	}
	return obj, nil
}

func transformPod(pod *corev1.Pod) *corev1.Pod {
	// Strip ManagedFields
	pod.ManagedFields = nil

	// Strip annotations except kubesage-*
	if pod.Annotations != nil {
		filtered := make(map[string]string)
		for k, v := range pod.Annotations {
			if strings.HasPrefix(k, "kubesage") {
				filtered[k] = v
			}
		}
		if len(filtered) == 0 {
			pod.Annotations = nil
		} else {
			pod.Annotations = filtered
		}
	}

	// Strip OwnerReferences
	pod.OwnerReferences = nil

	// Keep only needed container fields (name + resources)
	for i := range pod.Spec.Containers {
		pod.Spec.Containers[i].Command = nil
		pod.Spec.Containers[i].Args = nil
		pod.Spec.Containers[i].Env = nil
		pod.Spec.Containers[i].EnvFrom = nil
		pod.Spec.Containers[i].VolumeMounts = nil
		pod.Spec.Containers[i].Ports = nil
		pod.Spec.Containers[i].LivenessProbe = nil
		pod.Spec.Containers[i].ReadinessProbe = nil
		pod.Spec.Containers[i].StartupProbe = nil
		pod.Spec.Containers[i].SecurityContext = nil
	}
	pod.Spec.InitContainers = nil
	pod.Spec.Volumes = nil

	// Keep Status.Phase and ContainerStatuses (for restart count)
	pod.Status.Conditions = nil

	return pod
}

func transformNode(node *corev1.Node) *corev1.Node {
	// Strip ManagedFields
	node.ManagedFields = nil
	// Strip Annotations
	node.Annotations = nil
	// Keep: Name, Status.Conditions, Status.Allocatable
	node.Status.Images = nil
	node.Spec.Taints = nil
	return node
}

func transformDeployment(dep *appsv1.Deployment) *appsv1.Deployment {
	// Strip ManagedFields
	dep.ManagedFields = nil
	// Strip Annotations
	dep.Annotations = nil
	// Strip Spec.Template (we only need Spec.Replicas)
	dep.Spec.Template = corev1.PodTemplateSpec{}
	dep.Spec.Selector = nil
	dep.Spec.Strategy = appsv1.DeploymentStrategy{}
	return dep
}

// quantityToFloat64 converts a K8s resource.Quantity to float64 (cores for CPU).
func quantityToFloat64(q *resource.Quantity) float64 {
	if q == nil || q.IsZero() {
		return 0
	}
	return q.AsApproximateFloat64()
}
