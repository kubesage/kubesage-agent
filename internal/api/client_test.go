package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHeartbeat_SendsCorrectRequest(t *testing.T) {
	var gotPath string
	var gotMethod string
	var gotBody HeartbeatRequest
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-agent-token", "cluster-123")
	err := client.Heartbeat(context.Background(), "1.0.0", 3, 42)
	require.NoError(t, err)

	assert.Equal(t, "/api/v1/clusters/cluster-123/heartbeat", gotPath)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "Bearer test-agent-token", gotAuth)
	assert.Equal(t, 3, gotBody.NodeCount)
	assert.Equal(t, 42, gotBody.PodCount)
}

func TestHeartbeat_IncludesAgentVersion(t *testing.T) {
	var gotBody HeartbeatRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok", "cid")
	err := client.Heartbeat(context.Background(), "2.1.0", 1, 5)
	require.NoError(t, err)
	assert.Equal(t, "2.1.0", gotBody.AgentVersion)
}

func TestReportEvent_SendsCorrectRequest(t *testing.T) {
	var gotPath string
	var gotBody ClusterEvent

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok", "cluster-456")
	event := ClusterEvent{
		Type:    "warning",
		Message: "pod crash loop",
		Source:  "kubelet",
	}
	err := client.ReportEvent(context.Background(), event)
	require.NoError(t, err)

	assert.Equal(t, "/api/v1/clusters/cluster-456/events", gotPath)
	assert.Equal(t, "warning", gotBody.Type)
	assert.Equal(t, "pod crash loop", gotBody.Message)
	assert.Equal(t, "kubelet", gotBody.Source)
}

func TestClient_ErrorOnNon2xxResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok", "cid")
	err := client.Heartbeat(context.Background(), "1.0.0", 1, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestClient_ErrorOnConnectionRefused(t *testing.T) {
	// Use a URL that will definitely refuse connection
	client := NewClient("http://127.0.0.1:1", "tok", "cid")
	err := client.Heartbeat(context.Background(), "1.0.0", 1, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sending request")
}
