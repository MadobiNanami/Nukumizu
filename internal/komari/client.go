package komari

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"nukumizu-backend/config"
	"nukumizu-backend/global"
	"nukumizu-backend/internal/node"
	"nukumizu-backend/postLog"
)

// taskEchoEnabled reports whether Komari task progress should be echoed to the
// log: debug mode plus the showKomariTaskEcho toggle. The configuration is read
// once per call so both flags come from the same reload.
func taskEchoEnabled() bool {
	cfg := config.Current()
	return cfg != nil && cfg.System.DebugMode && cfg.Debug.ShowKomariTaskEcho
}

// NodeInfo represents a single node as returned by Komari's
// common:getNodes RPC2 method.
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
	IPv4             string  `json:"ipv4"`
	IPv6             string  `json:"ipv6"`
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
	Weight           int     `json:"weight"`
	Price            int     `json:"price"`
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
	// Normalize away a trailing slash so paths are appended as "/api/..." and
	// never become "//api/..." (which Komari's router rejects with 404).
	baseURL = strings.TrimRight(baseURL, "/")
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

// retryMaxAttempts bounds how many times a request is tried before giving up.
const retryMaxAttempts = 6

// retryBackoff is the linear backoff step between attempts: the n-th retry
// waits n*retryBackoff.
const retryBackoff = 500 * time.Millisecond

// retryableError marks a failure worth retrying: transport errors, rate
// limiting and 5xx responses. Rejected credentials, 4xx responses and
// malformed payloads are not retryable, so callers fail fast instead of
// hammering Komari (which could trip account lockout or rate limits).
type retryableError struct {
	err error
}

func (e *retryableError) Error() string { return e.err.Error() }

func (e *retryableError) Unwrap() error { return e.err }

func retryableErrorf(format string, args ...any) error {
	return &retryableError{err: fmt.Errorf(format, args...)}
}

// isRetryableError reports whether err was produced by a failure that another
// attempt may resolve.
func isRetryableError(err error) bool {
	var retryable *retryableError
	return errors.As(err, &retryable)
}

// doWithRetry runs fn until it succeeds, fails with a non-retryable error, or
// exhausts retryMaxAttempts attempts. what names the operation in logs and in
// the returned error.
func doWithRetry(what string, fn func() error) error {
	var lastErr error
	for attempt := 1; ; attempt++ {
		if err := fn(); err != nil {
			lastErr = err
			if !isRetryableError(err) || attempt == retryMaxAttempts {
				return fmt.Errorf("%s failed after %d attempt(s): %w", what, attempt, lastErr)
			}
			postLog.Warning(fmt.Sprintf("%s attempt %d/%d failed, retrying: %v", what, attempt, retryMaxAttempts, err))
			time.Sleep(time.Duration(attempt) * retryBackoff)
			continue
		}
		return nil
	}
}

// truncateBody keeps error messages readable when a proxy or gateway answers
// with an unexpected (possibly HTML) payload.
func truncateBody(body []byte) string {
	const limit = 512
	if len(body) > limit {
		return string(body[:limit]) + "..."
	}
	return strings.TrimSpace(string(body))
}

// Login authenticates to Komari and stores the session token cookie. Transport
// errors, 429 and 5xx responses are retried with linear backoff; credential or
// protocol failures return immediately.
func (c *Client) Login(username, password string) error {
	postLog.Info("Logging into Komari Dashboard at " + c.baseURL)

	err := doWithRetry("komari login", func() error {
		return c.loginOnce(username, password)
	})
	if err != nil {
		// Drop any stale token so callers cannot keep using a dead session.
		c.mu.Lock()
		c.sessionToken = ""
		c.mu.Unlock()
		return err
	}

	postLog.Info("Successfully logged into Komari Dashboard")
	return nil
}

// loginOnce performs a single login round trip. The response body is closed
// before returning so retries never accumulate open connections.
func (c *Client) loginOnce(username, password string) error {
	body := map[string]string{
		"username": username,
		"password": password,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal komari login request: %w", err)
	}

	resp, err := c.httpClient.Post(c.baseURL+"/api/login", "application/json", bytes.NewReader(bodyJSON))
	if err != nil {
		return retryableErrorf("komari login request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return retryableErrorf("failed to read komari login response: %w", err)
	}

	switch {
	case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode >= 500:
		return retryableErrorf("komari login request failed: unexpected status code %d: %s", resp.StatusCode, truncateBody(respBody))
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("komari login request failed: unexpected status code %d: %s", resp.StatusCode, truncateBody(respBody))
	}

	var kr KomariResponse
	if err := json.Unmarshal(respBody, &kr); err != nil {
		if config.IsDebugMode() {
			return fmt.Errorf("failed to parse komari login response: %w.\nResponse: %s", err, truncateBody(respBody))
		}
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
			return nil
		}
	}

	return fmt.Errorf("komari login response missing session_token cookie")
}

// GetSessionToken returns the current session token (empty string if not logged in).
// The WebSocket client uses it to authenticate the /api/rpc2 connection.
func (c *Client) GetSessionToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionToken
}

// FetchNodes retrieves all nodes from Komari via the JSON-RPC2 endpoint.
// Transport errors, 429 and 5xx responses are retried with linear backoff;
// RPC-level errors and malformed payloads fail immediately.
func (c *Client) FetchNodes() ([]NodeInfo, error) {
	var nodes []NodeInfo
	err := doWithRetry("komari nodes fetch", func() error {
		var err error
		nodes, err = c.fetchNodesOnce()
		return err
	})
	if err != nil {
		return nil, err
	}

	// Persist the fetched node UUIDs to bot_node_config.json on every fetch
	// (startup login, WebSocket reconnect and periodic refresh all funnel
	// through FetchNodes) so the file always reflects the nodes Komari
	// currently manages. A persistence failure is only logged: the node data
	// itself was fetched successfully and must not be discarded over a disk
	// write problem.
	uuids := make([]string, len(nodes))
	for i, n := range nodes {
		uuids[i] = n.UUID
	}
	if err := config.SaveBotNodeConfig(global.ConfigPath.BotNodeConfig, uuids); err != nil {
		postLog.Warning("Failed to save bot node config: " + err.Error())
	}

	// postLog.Debug(fmt.Sprintf("Fetched %d nodes from Komari", len(nodes)))
	return nodes, nil
}

// fetchNodesOnce performs a single nodes RPC round trip. The response body is
// closed before returning so retries never accumulate open connections.
func (c *Client) fetchNodesOnce() ([]NodeInfo, error) {
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "common:getNodes",
		"params":  map[string]any{},
		"id":      1,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal nodes RPC request: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL+"/api/rpc2", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create nodes request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, retryableErrorf("komari nodes request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, retryableErrorf("failed to read komari nodes response: %w", err)
	}

	switch {
	case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode >= 500:
		return nil, retryableErrorf("komari nodes request failed: unexpected status code %d: %s", resp.StatusCode, truncateBody(respBody))
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("komari nodes request failed: unexpected status code %d: %s", resp.StatusCode, truncateBody(respBody))
	}

	var rpcResp jsonRpcResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("failed to parse komari nodes RPC response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("komari nodes RPC error: %s", rpcResp.Error.Message)
	}

	var nodeMap map[string]NodeInfo
	if err := json.Unmarshal(rpcResp.Result, &nodeMap); err != nil {
		return nil, fmt.Errorf("failed to parse komari nodes data: %w", err)
	}

	nodes := make([]NodeInfo, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// BuildNodeListData converts the Komari node list into the tracker's input,
// carrying each node's static Info metadata alongside its name. It is used
// whenever the node list is fetched (startup login, reconnect, periodic refresh)
// so the tracker keeps the Info of every node up to date.
func BuildNodeListData(nodes []NodeInfo) map[string]node.NodeListEntry {
	entries := make(map[string]node.NodeListEntry, len(nodes))
	for _, n := range nodes {
		info := &node.Info{}
		info.OS.Name = n.OS
		info.OS.KernelVersion = n.KernelVersion
		info.CPU.Model = n.CPUName
		info.CPU.Cores = n.CPUCores
		info.CPU.Arch = n.Arch
		info.RAM.Total = n.MemTotal
		info.SWAP.Total = n.SwapTotal
		info.Disk.Total = n.DiskTotal
		info.BillingCycle = fmt.Sprintf("%d", n.BillingCycle)
		info.Price = n.Price
		info.Group = n.Group
		info.Tags = n.Tags
		info.IPv4 = n.IPv4
		info.IPv6 = n.IPv6

		entries[n.UUID] = node.NodeListEntry{
			Name: n.Name,
			Info: info,
		}
	}
	return entries
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

	if taskEchoEnabled() {
		postLog.Debug(fmt.Sprintf("Created Komari task %s for %d clients", result.TaskID, len(uuids)))
	}
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
	if taskEchoEnabled() {
		postLog.Debug(fmt.Sprintf("Polling for Komari task %s results...", taskID))
	}

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
				if taskEchoEnabled() {
					postLog.Info(fmt.Sprintf("Task %s completed with %d results", taskID, len(results)))
				}
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
