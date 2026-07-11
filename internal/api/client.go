package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client is a REST HTTP client for communicating with the KubeSage Go API.
// It reports heartbeats and cluster events against the tenant-scoped API
// contract (/api/v1/tenants/{tenantId}/clusters/{clusterId}/...).
type Client struct {
	baseURL    string
	agentToken string
	tenantID   string
	clusterID  string
	http       *http.Client
}

// NewClient creates an API client configured for a specific tenant + cluster.
func NewClient(baseURL, agentToken, tenantID, clusterID string) *Client {
	return &Client{
		baseURL:    baseURL,
		agentToken: agentToken,
		tenantID:   tenantID,
		clusterID:  clusterID,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// HeartbeatRequest is the JSON body sent to the heartbeat endpoint.
// Field tags are snake_case to match the Go API ClusterHeartbeatRequest schema.
type HeartbeatRequest struct {
	AgentVersion string `json:"agent_version"`
	NodeCount    int    `json:"node_count"`
	PodCount     int    `json:"pod_count"`
}

// ClusterEvent is the JSON body sent to the events endpoint. It matches the
// Go API CreateClusterEventRequest schema; severity and occurred_at are required.
type ClusterEvent struct {
	Type       string `json:"type"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	OccurredAt string `json:"occurred_at"`
}

// Heartbeat sends a heartbeat to the Go API with current cluster metrics.
func (c *Client) Heartbeat(ctx context.Context, agentVersion string, nodeCount, podCount int) error {
	body := HeartbeatRequest{
		AgentVersion: agentVersion,
		NodeCount:    nodeCount,
		PodCount:     podCount,
	}
	return c.post(ctx, fmt.Sprintf("/api/v1/tenants/%s/clusters/%s/heartbeat", c.tenantID, c.clusterID), body)
}

// ReportEvent sends a cluster event to the Go API.
func (c *Client) ReportEvent(ctx context.Context, event ClusterEvent) error {
	return c.post(ctx, fmt.Sprintf("/api/v1/tenants/%s/clusters/%s/events", c.tenantID, c.clusterID), event)
}

func (c *Client) post(ctx context.Context, path string, body interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.agentToken)
	// RequireTenantForPrefix (middleware/tenant.go) 400s tenant-scoped routes
	// without a valid X-Tenant-ID UUID header.
	req.Header.Set("X-Tenant-ID", c.tenantID)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d for %s", resp.StatusCode, path)
	}
	return nil
}
