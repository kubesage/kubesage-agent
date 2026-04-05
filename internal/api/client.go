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
// It handles heartbeat reporting and cluster event creation.
type Client struct {
	baseURL    string
	agentToken string
	clusterID  string
	http       *http.Client
}

// NewClient creates an API client configured for a specific cluster.
func NewClient(baseURL, agentToken, clusterID string) *Client {
	return &Client{
		baseURL:    baseURL,
		agentToken: agentToken,
		clusterID:  clusterID,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// HeartbeatRequest is the JSON body sent to the heartbeat endpoint.
type HeartbeatRequest struct {
	AgentVersion string `json:"agentVersion"`
	NodeCount    int    `json:"nodeCount"`
	PodCount     int    `json:"podCount"`
}

// ClusterEvent is the JSON body sent to the events endpoint.
type ClusterEvent struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Source  string `json:"source"`
}

// Heartbeat sends a heartbeat to the Go API with current cluster metrics.
func (c *Client) Heartbeat(ctx context.Context, agentVersion string, nodeCount, podCount int) error {
	body := HeartbeatRequest{
		AgentVersion: agentVersion,
		NodeCount:    nodeCount,
		PodCount:     podCount,
	}
	return c.post(ctx, fmt.Sprintf("/api/v1/clusters/%s/heartbeat", c.clusterID), body)
}

// ReportEvent sends a cluster event to the Go API.
func (c *Client) ReportEvent(ctx context.Context, event ClusterEvent) error {
	return c.post(ctx, fmt.Sprintf("/api/v1/clusters/%s/events", c.clusterID), event)
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
