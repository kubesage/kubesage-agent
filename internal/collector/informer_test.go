package collector

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric/noop"
	"go.uber.org/zap/zaptest"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kubesage/kubesage-agent/internal/metrics"
)

func TestInformerCollector_StartAndCollect(t *testing.T) {
	replicas := int32(3)

	clientset := fake.NewSimpleClientset(
		// 3 pods in different phases
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-running",
				Namespace: "default",
				ManagedFields: []metav1.ManagedFieldsEntry{
					{Manager: "test"},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "app",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{RestartCount: 2},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-pending",
				Namespace: "default",
			},
			Status: corev1.PodStatus{Phase: corev1.PodPending},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-failed",
				Namespace: "kube-system",
			},
			Status: corev1.PodStatus{Phase: corev1.PodFailed},
		},
		// 2 nodes
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
				ManagedFields: []metav1.ManagedFieldsEntry{
					{Manager: "kubelet"},
				},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
				},
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("8"),
					corev1.ResourceMemory: resource.MustParse("32Gi"),
				},
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
		// 1 deployment
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web",
				Namespace: "default",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
			},
			Status: appsv1.DeploymentStatus{
				AvailableReplicas: 2,
				Replicas:          3,
			},
		},
	)

	logger := zaptest.NewLogger(t)
	meter := noop.Meter{}
	inst, err := metrics.NewInstruments(meter)
	require.NoError(t, err)

	ic := NewInformerCollector(clientset, inst, logger, "test-cluster", 30*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = ic.Start(ctx)
	require.NoError(t, err)

	// Collect should not panic
	ic.Collect(ctx)

	// Verify node names
	nodeNames := ic.NodeNames()
	assert.Len(t, nodeNames, 2)
	assert.Contains(t, nodeNames, "node-1")
	assert.Contains(t, nodeNames, "node-2")
}

func TestTransformPod_StripsManagedFields(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl"},
				{Manager: "kubelet"},
			},
			Annotations: map[string]string{
				"kubectl.kubernetes.io/last-applied": "{}",
				"kubesage-version":                   "1.0",
			},
			OwnerReferences: []metav1.OwnerReference{
				{Name: "replicaset-1"},
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "app",
					Command: []string{"/bin/sh"},
					Args:    []string{"-c", "sleep infinity"},
					Env:     []corev1.EnvVar{{Name: "FOO", Value: "bar"}},
				},
			},
			InitContainers: []corev1.Container{
				{Name: "init"},
			},
			Volumes: []corev1.Volume{
				{Name: "data"},
			},
		},
	}

	result := transformPod(pod)

	// ManagedFields stripped
	assert.Nil(t, result.ManagedFields)
	// Non-kubesage annotations stripped
	assert.Equal(t, map[string]string{"kubesage-version": "1.0"}, result.Annotations)
	// OwnerReferences stripped
	assert.Nil(t, result.OwnerReferences)
	// Container unnecessary fields stripped
	assert.Nil(t, result.Spec.Containers[0].Command)
	assert.Nil(t, result.Spec.Containers[0].Args)
	assert.Nil(t, result.Spec.Containers[0].Env)
	// InitContainers and Volumes stripped
	assert.Nil(t, result.Spec.InitContainers)
	assert.Nil(t, result.Spec.Volumes)
}

func TestTransformNode_StripsManagedFields(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubelet"},
			},
			Annotations: map[string]string{
				"node.alpha.kubernetes.io/ttl": "0",
			},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{{Key: "special"}},
		},
		Status: corev1.NodeStatus{
			Images: []corev1.ContainerImage{{Names: []string{"nginx"}}},
		},
	}

	result := transformNode(node)

	assert.Nil(t, result.ManagedFields)
	assert.Nil(t, result.Annotations)
	assert.Nil(t, result.Spec.Taints)
	assert.Nil(t, result.Status.Images)
}

func TestTransformDeployment_StripsTemplate(t *testing.T) {
	replicas := int32(5)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web",
			Namespace: "default",
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl"},
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app", Image: "nginx"}},
				},
			},
		},
	}

	result := transformDeployment(dep)

	assert.Nil(t, result.ManagedFields)
	assert.Nil(t, result.Spec.Selector)
	assert.Equal(t, int32(5), *result.Spec.Replicas)
	// Template should be zeroed
	assert.Empty(t, result.Spec.Template.Spec.Containers)
}

func TestTransformObject_PassthroughUnknown(t *testing.T) {
	type unknown struct{}
	obj := &unknown{}
	result, err := transformObject(obj)
	assert.NoError(t, err)
	assert.Equal(t, obj, result)
}
