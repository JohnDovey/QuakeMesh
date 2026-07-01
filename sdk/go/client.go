// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.12 - Phase 10: HTTP client for the local mesh-sdk daemon API.

package sdk

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// HTTPClient talks to QuakeMeshHub's or QuakeMesh's local daemon API.
type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
	session    *Session
	token      string
}

// NewUnixClient connects over a Unix domain socket (Hub default).
func NewUnixClient(socketPath string) *HTTPClient {
	return &HTTPClient{
		baseURL: "http://unix",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socketPath)
				},
			},
		},
	}
}

// NewTCPClient connects over loopback TCP (tests and Android).
func NewTCPClient(hostPort string) *HTTPClient {
	return &HTTPClient{
		baseURL:    "http://" + hostPort,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Register implements Client.
func (c *HTTPClient) Register(appID, appName, appVersion string, capabilities []string) (*Session, error) {
	body, _ := json.Marshal(map[string]any{
		"app_id": appID, "app_name": appName, "app_version": appVersion,
		"capabilities": capabilities,
	})
	resp, err := c.post("/v1/register", body, "")
	if err != nil {
		return nil, err
	}
	var out struct {
		SessionToken string `json:"session_token"`
		NodeID       string `json:"node_id"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		return nil, err
	}
	c.token = out.SessionToken
	nodeID, _ := hex.DecodeString(out.NodeID)
	c.session = &Session{
		AppID: appID, AppName: appName, AppVersion: appVersion, NodeID: nodeID,
	}
	return c.session, nil
}

// Send implements Client.
func (c *HTTPClient) Send(session *Session, destNodeID []byte, payload []byte) error {
	if c.token == "" {
		return fmt.Errorf("sdk: not registered")
	}
	body, _ := json.Marshal(map[string]string{
		"dest_node_id": hex.EncodeToString(destNodeID),
		"payload_b64":  base64.StdEncoding.EncodeToString(payload),
	})
	_, err := c.post("/v1/send", body, c.token)
	return err
}

// Receive implements Client.
func (c *HTTPClient) Receive(session *Session) (<-chan []byte, error) {
	ch := make(chan []byte, 8)
	go func() {
		defer close(ch)
		for {
			payload, ok, err := c.pollReceive()
			if err != nil || !ok {
				return
			}
			ch <- payload
		}
	}()
	return ch, nil
}

func (c *HTTPClient) pollReceive() ([]byte, bool, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/v1/receive", nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("X-Mesh-Session", c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		time.Sleep(200 * time.Millisecond)
		return nil, false, nil
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}
	var out struct {
		PayloadB64 string `json:"payload_b64"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, false, err
	}
	raw, err := base64.StdEncoding.DecodeString(out.PayloadB64)
	if err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

// Publish implements Client.
func (c *HTTPClient) Publish(session *Session, topic string, payload []byte) error {
	body, _ := json.Marshal(map[string]string{
		"topic": topic, "payload_b64": base64.StdEncoding.EncodeToString(payload),
	})
	_, err := c.post("/v1/publish", body, c.token)
	return err
}

// Subscribe implements Client.
func (c *HTTPClient) Subscribe(session *Session, topic string) (<-chan []byte, error) {
	ch := make(chan []byte, 8)
	go func() {
		defer close(ch)
		for {
			req, err := http.NewRequest(http.MethodGet, c.baseURL+"/v1/subscribe?topic="+topic+"&timeout=25s", nil)
			if err != nil {
				return
			}
			req.Header.Set("X-Mesh-Session", c.token)
			resp, err := c.httpClient.Do(req)
			if err != nil {
				return
			}
			if resp.StatusCode == http.StatusNoContent {
				resp.Body.Close()
				continue
			}
			data, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return
			}
			var out struct {
				PayloadB64 string `json:"payload_b64"`
			}
			if json.Unmarshal(data, &out) != nil {
				continue
			}
			raw, err := base64.StdEncoding.DecodeString(out.PayloadB64)
			if err != nil {
				continue
			}
			ch <- raw
		}
	}()
	return ch, nil
}

// DiscoverPeers implements Client.
func (c *HTTPClient) DiscoverPeers(appID string, versionConstraint string) ([][]byte, error) {
	url := fmt.Sprintf("%s/v1/discover-peers?app_id=%s", c.baseURL, appID)
	if versionConstraint != "" {
		url += "&version_constraint=" + versionConstraint
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Mesh-Session", c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var out struct {
		Peers []string `json:"peers"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	peers := make([][]byte, 0, len(out.Peers))
	for _, hexID := range out.Peers {
		raw, err := hex.DecodeString(hexID)
		if err != nil {
			continue
		}
		peers = append(peers, raw)
	}
	return peers, nil
}

func (c *HTTPClient) post(path string, body []byte, token string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Mesh-Session", token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sdk: %s: %s", path, string(data))
	}
	return data, nil
}
