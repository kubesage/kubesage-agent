package collector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric/noop"
	"go.uber.org/zap/zaptest"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/kubesage/kubesage-agent/internal/metrics"
)

func TestKubeletCollector_Collect(t *testing.T) {
	cpuNano := uint64(500_000_000) // 0.5 cores
	memBytes := uint64(1_073_741_824) // 1 GiB
	podCPU := uint64(100_000_000)  // 0.1 cores
	podMem := uint64(268_435_456)  // 256 MiB

	summary := Summary{
		Node: NodeStats{
			CPU:    &CPUStats{UsageNanoCores: &cpuNano},
			Memory: &MemoryStats{UsageBytes: &memBytes},
		},
		Pods: []PodStats{
			{
				PodRef: PodRef{Name: "web-abc123", Namespace: "default"},
				CPU:    &CPUStats{UsageNanoCores: &podCPU},
				Memory: &MemoryStats{UsageBytes: &podMem},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expect path like /api/v1/nodes/{node}/proxy/stats/summary
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(summary)
	}))
	defer server.Close()

	clientset, err := kubernetes.NewForConfig(&rest.Config{
		Host: server.URL,
	})
	require.NoError(t, err)

	logger := zaptest.NewLogger(t)
	meter := noop.Meter{}
	inst, err := metrics.NewInstruments(meter)
	require.NoError(t, err)

	kc := NewKubeletCollector(clientset, inst, logger, "test-cluster")

	ctx := context.Background()
	// Should not panic and should process the mock data
	kc.Collect(ctx, []string{"node-1"})
}

func TestKubeletCollector_HandlesForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"forbidden"}`))
	}))
	defer server.Close()

	clientset, err := kubernetes.NewForConfig(&rest.Config{
		Host: server.URL,
	})
	require.NoError(t, err)

	logger := zaptest.NewLogger(t)
	meter := noop.Meter{}
	inst, err := metrics.NewInstruments(meter)
	require.NoError(t, err)

	kc := NewKubeletCollector(clientset, inst, logger, "test-cluster")

	ctx := context.Background()
	// Should log warning but not panic
	kc.Collect(ctx, []string{"node-1"})
}

func TestKubeletCollector_HandlesNilStats(t *testing.T) {
	// Test with nil CPU/Memory stats -- should not panic
	summary := Summary{
		Node: NodeStats{
			CPU:    nil,
			Memory: nil,
		},
		Pods: []PodStats{
			{
				PodRef: PodRef{Name: "pod-1", Namespace: "default"},
				CPU:    nil,
				Memory: nil,
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(summary)
	}))
	defer server.Close()

	clientset, err := kubernetes.NewForConfig(&rest.Config{
		Host: server.URL,
	})
	require.NoError(t, err)

	logger := zaptest.NewLogger(t)
	meter := noop.Meter{}
	inst, err := metrics.NewInstruments(meter)
	require.NoError(t, err)

	kc := NewKubeletCollector(clientset, inst, logger, "test-cluster")
	kc.Collect(context.Background(), []string{"node-1"})
}

func TestKubeletCollector_MaxWorkers(t *testing.T) {
	kc := NewKubeletCollector(nil, nil, nil, "test")
	assert.Equal(t, 10, kc.maxWorkers)
}
