package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHeartbeat_SendsCorrectRequest(t *testing.T) {
	var gotPath string
	var gotMethod string
	var gotAuth string
	var gotTenant string
	var gotBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		gotTenant = r.Header.Get("X-Tenant-ID")
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-agent-token", "tenant-abc", "cluster-123")
	err := client.Heartbeat(context.Background(), "1.0.0", 3, 42)
	require.NoError(t, err)

	// Tenant-scoped route (Pitfall 3): /api/v1/tenants/{tenantID}/clusters/{clusterID}/heartbeat
	assert.Equal(t, "/api/v1/tenants/tenant-abc/clusters/cluster-123/heartbeat", gotPath)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "Bearer test-agent-token", gotAuth)
	// X-Tenant-ID is required by RequireTenantForPrefix (middleware/tenant.go).
	assert.Equal(t, "tenant-abc", gotTenant)
	// Body keys are snake_case to match the Go API ClusterHeartbeatRequest schema.
	assert.Contains(t, gotBody, "agent_version")
	assert.Contains(t, gotBody, "node_count")
	assert.Contains(t, gotBody, "pod_count")
	assert.EqualValues(t, 3, gotBody["node_count"])
	assert.EqualValues(t, 42, gotBody["pod_count"])
}

func TestHeartbeat_IncludesAgentVersion(t *testing.T) {
	var gotBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok", "tid", "cid")
	err := client.Heartbeat(context.Background(), "2.1.0", 1, 5)
	require.NoError(t, err)
	assert.Equal(t, "2.1.0", gotBody["agent_version"])
}

func TestReportEvent_SendsCorrectRequest(t *testing.T) {
	var gotPath string
	var gotTenant string
	var gotBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotTenant = r.Header.Get("X-Tenant-ID")
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok", "tenant-xyz", "cluster-456")
	event := ClusterEvent{
		Type:       "warning",
		Severity:   "high",
		Message:    "pod crash loop",
		OccurredAt: time.Now().UTC().Format(time.RFC3339),
	}
	err := client.ReportEvent(context.Background(), event)
	require.NoError(t, err)

	// Tenant-scoped events route.
	assert.Equal(t, "/api/v1/tenants/tenant-xyz/clusters/cluster-456/events", gotPath)
	assert.Equal(t, "tenant-xyz", gotTenant)
	// CreateClusterEventRequest requires type/severity/message/occurred_at; `source` is dropped.
	assert.Equal(t, "warning", gotBody["type"])
	assert.Equal(t, "high", gotBody["severity"])
	assert.Equal(t, "pod crash loop", gotBody["message"])
	assert.Contains(t, gotBody, "occurred_at")
	assert.NotContains(t, gotBody, "source")
}

func TestClient_ErrorOnNon2xxResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok", "tid", "cid")
	err := client.Heartbeat(context.Background(), "1.0.0", 1, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestClient_ErrorOnConnectionRefused(t *testing.T) {
	// Use a URL that will definitely refuse connection
	client := NewClient("http://127.0.0.1:1", "tok", "tid", "cid")
	err := client.Heartbeat(context.Background(), "1.0.0", 1, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sending request")
}
