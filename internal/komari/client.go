package komari

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"
	"time"

	"nukumizu-backend/postLog"
)

// NodeInfo represents a single node from Komari's /api/nodes endpoint.
type NodeInfo struct {
	UUID             string  `json:"uuid"`
	Name             string  `json:"name"`
	CPUName          string  `json:"cpu_name"`
	Virtualization   string  `json:"virtualization"`
	Arch             string  `json:"arch"`
	CPUCores         int     `json:"cpu_cores"`
	CPUPhysicalCores int     `json:"cpu_physical_cores"`
	OS               string  `json:"os"`
	KernelVersion    string  `json:"kernel_version"`
	GPUName          string  `json:"gpu_name"`
	Region           string  `json:"region"`
	MemTotal         int64   `json:"mem_total"`
	SwapTotal        int64   `json:"swap_total"`
	DiskTotal        int64   `json:"disk_total"`
	Weight           float64 `json:"weight"`
	Price            float64 `json:"price"`
	BillingCycle     int     `json:"billing_cycle"`
	AutoRenewal      bool    `json:"auto_renewal"`
	Currency         string  `json:"currency"`
	ExpiredAt        *string `json:"expired_at"`
	Group            string  `json:"group"`
	Tags             string  `json:"tags"`
	Hidden           bool    `json:"hidden"`
	TrafficLimit     int64   `json:"traffic_limit"`
	TrafficLimitType string  `json:"traffic_limit_type"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

// TaskResult holds the result of a task execution from Komari.
type TaskResult struct {
	TaskID     string     `json:"task_id"`
	Client     string     `json:"client"`
	ClientInfo ClientInfo `json:"client_info"`
	Result     string     `json:"result"`
	ExitCode   int        `json:"exit_code"`
	FinishedAt string     `json:"finished_at"`
	CreatedAt  string     `json:"created_at"`
}

// ClientInfo holds client information returned with task results.
type ClientInfo struct {
	Name             string `json:"name"`
	CPUName          string `json:"cpu_name"`
	Virtualization   string `json:"virtualization"`
	Arch             string `json:"arch"`
	CPUCores         int    `json:"cpu_cores"`
	CPUPhysicalCores int    `json:"cpu_physical_cores"`
	OS               string `json:"os"`
	KernelVersion    string `json:"kernel_version"`
	GPUName          string `json:"gpu_name"`
	Region           string `json:"region"`
	MemTotal         int64  `json:"mem_total"`
	SwapTotal        int64  `json:"swap_total"`
	DiskTotal        int64  `json:"disk_total"`
	Weight           int    `json:"weight"`
	Price            int    `json:"price"`
	BillingCycle     int    `json:"billing_cycle"`
	AutoRenewal      bool   `json:"auto_renewal"`
	Currency         string `json:"currency"`
	ExpiredAt        *string `json:"expired_at"`
	Group            string `json:"group"`
	Tags             string `json:"tags"`
	Hidden           bool   `json:"hidden"`
	TrafficLimit     int64  `json:"traffic_limit"`
	TrafficLimitType string `json:"traffic_limit_type"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// KomariResponse is the standard response envelope from Komari API.
type KomariResponse struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// Client is the HTTP client for interacting with the Komari Dashboard API.
type Client struct {
	baseURL      string
	sessionToken string
	httpClient   *http.Client
	mu           sync.RWMutex
}

// NewClient creates a new Komari API client.
func NewClient(baseURL string) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
	}
}

// Login authenticates to Komari and stores the session token cookie.
func (c *Client) Login(username, password string) error {
	postLog.Info("Logging into Komari Dashboard at " + c.baseURL)

	body := map[string]string{
		"username": username,
		"password": password,
	}
	bodyJSON, _ := json.Marshal(body)

	resp, err := c.httpClient.Post(c.baseURL+"/api/login", "application/json", bytes.NewReader(bodyJSON))
	if err != nil {
		return fmt.Errorf("komari login request failed: %w", err)
	}
	defer resp.Body.Close()

	var kr KomariResponse
	if err := json.NewDecoder(resp.Body).Decode(&kr); err != nil {
		return fmt.Errorf("failed to parse komari login response: %w", err)
	}

	if kr.Status != "success" {
		return fmt.Errorf("komari login failed: %s", kr.Message)
	}

	// Extract session_token from cookies.
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "session_token" {
			c.mu.Lock()
			c.sessionToken = cookie.Value
			c.mu.Unlock()
			postLog.Info("Successfully logged into Komari Dashboard")
			return nil
		}
	}

	return fmt.Errorf("komari login response missing session_token cookie")
}

// getSessionToken returns the current session token.
func (c *Client) getSessionToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionToken
}

// FetchNodes retrieves all nodes from Komari.
func (c *Client) FetchNodes() ([]NodeInfo, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/nodes", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create nodes request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("komari nodes request failed: %w", err)
	}
	defer resp.Body.Close()

	var kr KomariResponse
	if err := json.NewDecoder(resp.Body).Decode(&kr); err != nil {
		return nil, fmt.Errorf("failed to parse komari nodes response: %w", err)
	}

	if kr.Status != "success" {
		return nil, fmt.Errorf("komari nodes request failed: %s", kr.Message)
	}

	var nodes []NodeInfo
	if err := json.Unmarshal(kr.Data, &nodes); err != nil {
		return nil, fmt.Errorf("failed to parse komari nodes data: %w", err)
	}

	postLog.Info(fmt.Sprintf("Fetched %d nodes from Komari", len(nodes)))
	return nodes, nil
}

// FetchRecent retrieves the recent status for a specific node.
func (c *Client) FetchRecent(uuid string) (json.RawMessage, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/recent/"+url.PathEscape(uuid), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create recent request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("komari recent request failed: %w", err)
	}
	defer resp.Body.Close()

	var kr KomariResponse
	if err := json.NewDecoder(resp.Body).Decode(&kr); err != nil {
		return nil, fmt.Errorf("failed to parse komari recent response: %w", err)
	}

	if kr.Status != "success" {
		return nil, fmt.Errorf("komari recent request failed: %s", kr.Message)
	}

	return kr.Data, nil
}

// ExecTask sends a command execution request to Komari and returns the task ID.
func (c *Client) ExecTask(uuids []string, command string) (string, error) {
	body := map[string]interface{}{
		"clients": uuids,
		"command": command,
	}
	bodyJSON, _ := json.Marshal(body)

	resp, err := c.httpClient.Post(c.baseURL+"/api/admin/task/exec", "application/json", bytes.NewReader(bodyJSON))
	if err != nil {
		return "", fmt.Errorf("komari task exec request failed: %w", err)
	}
	defer resp.Body.Close()

	var kr KomariResponse
	if err := json.NewDecoder(resp.Body).Decode(&kr); err != nil {
		return "", fmt.Errorf("failed to parse komari task exec response: %w", err)
	}

	if kr.Status != "success" {
		return "", fmt.Errorf("komari task exec failed: %s", kr.Message)
	}

	var result struct {
		Clients       []string `json:"clients"`
		QueuedClients []string `json:"queued_clients"`
		TaskID        string   `json:"task_id"`
	}
	if err := json.Unmarshal(kr.Data, &result); err != nil {
		return "", fmt.Errorf("failed to parse komari task exec data: %w", err)
	}

	postLog.Info(fmt.Sprintf("Created Komari task %s for %d clients", result.TaskID, len(uuids)))
	return result.TaskID, nil
}

// GetTaskResult retrieves the result of a task execution.
func (c *Client) GetTaskResult(taskID string) ([]TaskResult, bool, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/admin/task/" + url.PathEscape(taskID) + "/result")
	if err != nil {
		return nil, false, fmt.Errorf("komari task result request failed: %w", err)
	}
	defer resp.Body.Close()

	var kr KomariResponse
	if err := json.NewDecoder(resp.Body).Decode(&kr); err != nil {
		return nil, false, fmt.Errorf("failed to parse komari task result response: %w", err)
	}

	if kr.Status != "success" {
		return nil, false, fmt.Errorf("komari task result failed: %s", kr.Message)
	}

	var results []TaskResult
	if err := json.Unmarshal(kr.Data, &results); err != nil {
		return nil, false, fmt.Errorf("failed to parse komari task result data: %w", err)
	}

	// Check if all results have completed (result field is non-empty).
	allDone := true
	for _, r := range results {
		if r.Result == "" {
			allDone = false
			break
		}
	}

	return results, allDone, nil
}

// PollTaskResult polls for task results every 1 second until all results are
// available or 60 seconds have elapsed.
func (c *Client) PollTaskResult(taskID string) ([]TaskResult, error) {
	postLog.Info(fmt.Sprintf("Polling for Komari task %s results...", taskID))

	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("task execution timed out after 60 seconds")
		case <-ticker.C:
			results, done, err := c.GetTaskResult(taskID)
			if err != nil {
				return nil, err
			}
			if done {
				postLog.Info(fmt.Sprintf("Task %s completed with %d results", taskID, len(results)))
				return results, nil
			}
		}
	}
}

// GetHTTPClient returns the underlying HTTP client for use by other components.
func (c *Client) GetHTTPClient() *http.Client {
	return c.httpClient
}

// GetBaseURL returns the Komari base URL.
func (c *Client) GetBaseURL() string {
	return c.baseURL
}
