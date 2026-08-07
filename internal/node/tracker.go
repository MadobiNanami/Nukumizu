package node

import (
	"fmt"
	"sync"
	"time"

	"nukumizu-backend/postLog"
)

// Report represents the latest server status report from Komari WebSocket.
type Report struct {
	CPU struct {
		Usage float64 `json:"usage"`
	} `json:"cpu"`
	RAM struct {
		Total int64 `json:"total"`
		Used  int64 `json:"used"`
	} `json:"ram"`
	Swap struct {
		Total int64 `json:"total"`
		Used  int64 `json:"used"`
	} `json:"swap"`
	Load struct {
		Load1  float64 `json:"load1"`
		Load5  float64 `json:"load5"`
		Load15 float64 `json:"load15"`
	} `json:"load"`
	Disk struct {
		Total int64 `json:"total"`
		Used  int64 `json:"used"`
	} `json:"disk"`
	Network struct {
		Up        int64 `json:"up"`
		Down      int64 `json:"down"`
		TotalUp   int64 `json:"totalUp"`
		TotalDown int64 `json:"totalDown"`
	} `json:"network"`
	Connections struct {
		TCP int `json:"tcp"`
		UDP int `json:"udp"`
	} `json:"connections"`
	Uptime    int    `json:"uptime"`
	Process   int    `json:"process"`
	Message   string `json:"message"`
	UpdatedAt string `json:"updated_at"`
}

// Node holds all tracked information about a single server node.
type Node struct {
	UUID         string
	Name         string
	Online       bool
	LatestReport *Report
	LastUpdated  time.Time
}

// StatusChange represents a node status transition.
type StatusChange struct {
	Event     string // "Online" or "Offline"
	UUID      string
	Name      string
	Message   string
	OldOnline bool
	NewOnline bool
}

// StatusChangeCallback is called when a node's online status changes.
type StatusChangeCallback func(change StatusChange)

// Tracker maintains the in-memory state of all monitored nodes.
// All methods are thread-safe.
type Tracker struct {
	mu         sync.RWMutex
	nodes      map[string]*Node  // uuid → Node
	uuidToName map[string]string // uuid → name (from node list)
	onlineSet  map[string]bool   // which uuids are currently online
	callbacks  []StatusChangeCallback

	// Initial-refresh gating: status-change notifications are suppressed until
	// every node from the Komari node list has delivered a status update, so the
	// startup state is treated as a baseline rather than a flood of changes.
	knownUUIDs    map[string]bool // uuids from the Komari node list
	reportedUUIDs map[string]bool // node-list uuids that have reported status
	bootstrapDone bool            // true once the initial refresh has finished
}

var globalTracker *Tracker

// InitTracker initializes the global node tracker.
func InitTracker() {
	globalTracker = &Tracker{
		nodes:         make(map[string]*Node),
		uuidToName:    make(map[string]string),
		onlineSet:     make(map[string]bool),
		knownUUIDs:    make(map[string]bool),
		reportedUUIDs: make(map[string]bool),
	}
	postLog.Info("Node tracker initialized")
}

// GetTracker returns the global tracker instance.
func GetTracker() *Tracker {
	return globalTracker
}

// OnStatusChange registers a callback for status change events.
func (t *Tracker) OnStatusChange(cb StatusChangeCallback) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.callbacks = append(t.callbacks, cb)
}

// fireCallbacks notifies all registered callbacks of a status change.
// Must be called with the lock NOT held (it acquires RLocks as needed).
func (t *Tracker) fireCallbacks(change StatusChange) {
	t.mu.RLock()
	cbs := make([]StatusChangeCallback, len(t.callbacks))
	copy(cbs, t.callbacks)
	t.mu.RUnlock()

	for _, cb := range cbs {
		cb(change)
	}
}

// UpdateNodeList replaces the full node list and updates the uuid→name map.
// nodeNames maps UUID → node name from the Komari node list.
func (t *Tracker) UpdateNodeList(nodeNames map[string]string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// The node list defines the set of nodes to wait for during the initial
	// status refresh.
	t.knownUUIDs = make(map[string]bool, len(nodeNames))
	for uuid := range nodeNames {
		t.knownUUIDs[uuid] = true
	}

	t.uuidToName = make(map[string]string, len(nodeNames))
	for uuid, name := range nodeNames {
		t.uuidToName[uuid] = name
		// Ensure node entry exists.
		if _, exists := t.nodes[uuid]; !exists {
			t.nodes[uuid] = &Node{
				UUID:   uuid,
				Name:   name,
				Online: false,
			}
		} else {
			// Update name in case it changed.
			t.nodes[uuid].Name = name
		}
	}

	// postLog.Debug(fmt.Sprintf("Node list updated: %d nodes", len(nodeNames)))
}

// UpdateStatus processes a WebSocket status update from Komari.
// It returns a list of status changes (Online/Offline) that were detected.
func (t *Tracker) UpdateStatus(onlineUUIDs []string, reports map[string]Report) []StatusChange {
	t.mu.Lock()

	newOnlineSet := make(map[string]bool, len(onlineUUIDs))
	for _, uuid := range onlineUUIDs {
		newOnlineSet[uuid] = true
	}

	var changes []StatusChange

	// Detect newly online nodes.
	for uuid := range newOnlineSet {
		if !t.onlineSet[uuid] {
			name := t.uuidToName[uuid]
			changes = append(changes, StatusChange{
				Event:     "Online",
				UUID:      uuid,
				Name:      name,
				Message:   "Server came online",
				OldOnline: false,
				NewOnline: true,
			})
		}
		// Update or create node entry.
		node, exists := t.nodes[uuid]
		if !exists {
			node = &Node{UUID: uuid, Name: t.uuidToName[uuid]}
			t.nodes[uuid] = node
		}
		node.Online = true
		node.LastUpdated = time.Now()
	}

	// Detect newly offline nodes.
	for uuid := range t.onlineSet {
		if !newOnlineSet[uuid] {
			name := t.uuidToName[uuid]
			message := "Server went offline"
			if node, exists := t.nodes[uuid]; exists {
				node.Online = false
				node.LastUpdated = time.Now()
				if node.LatestReport != nil && node.LatestReport.Message != "" {
					message = node.LatestReport.Message
				}
			}
			changes = append(changes, StatusChange{
				Event:     "Offline",
				UUID:      uuid,
				Name:      name,
				Message:   message,
				OldOnline: true,
				NewOnline: false,
			})
		}
	}

	// Update report data for online nodes.
	for uuid, report := range reports {
		reportCopy := report
		node, exists := t.nodes[uuid]
		if !exists {
			node = &Node{UUID: uuid, Name: t.uuidToName[uuid]}
			t.nodes[uuid] = node
		}
		node.LatestReport = &reportCopy
		node.LastUpdated = time.Now()
	}

	t.onlineSet = newOnlineSet

	// Track which node-list nodes have delivered a status update. The initial
	// refresh is considered complete once every known node has reported at
	// least once (either online or offline).
	for _, uuid := range onlineUUIDs {
		if t.knownUUIDs[uuid] {
			t.reportedUUIDs[uuid] = true
		}
	}
	for uuid := range reports {
		if t.knownUUIDs[uuid] {
			t.reportedUUIDs[uuid] = true
		}
	}

	// Changes detected while the initial refresh is still in progress represent
	// the nodes' startup state rather than real transitions, so suppress them.
	suppressChanges := !t.bootstrapDone
	if !t.bootstrapDone && t.allKnownNodesReported() {
		t.bootstrapDone = true
		postLog.Info(fmt.Sprintf("All %d nodes refreshed their status; status notifications enabled", len(t.knownUUIDs)))
	}
	t.mu.Unlock()

	// Fire callbacks for each change (outside the lock).
	for _, change := range changes {
		if suppressChanges {
			continue
		}
		postLog.Info(fmt.Sprintf("Node status change: %s [%s] - %s", change.Name, change.UUID, change.Event))
		t.fireCallbacks(change)
	}

	return changes
}

// allKnownNodesReported returns true when every node from the Komari node list
// has delivered at least one status update. Must be called with the lock held.
func (t *Tracker) allKnownNodesReported() bool {
	for uuid := range t.knownUUIDs {
		if !t.reportedUUIDs[uuid] {
			return false
		}
	}
	return true
}

// CompleteBootstrap force-completes the initial status refresh gate. It is a
// safety net so that notifications cannot be blocked forever by a node that
// never reports a status. Calling it again is a no-op.
func (t *Tracker) CompleteBootstrap() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.bootstrapDone {
		t.bootstrapDone = true
		postLog.Info("Initial node status refresh timeout reached; status notifications enabled")
	}
}

// GetNode returns a copy of the node data for the given UUID.
func (t *Tracker) GetNode(uuid string) (*Node, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	node, exists := t.nodes[uuid]
	if !exists {
		return nil, false
	}
	// Return a copy to avoid data races.
	nodeCopy := *node
	if node.LatestReport != nil {
		reportCopy := *node.LatestReport
		nodeCopy.LatestReport = &reportCopy
	}
	return &nodeCopy, true
}

// GetAllNodes returns a copy of all tracked nodes.
func (t *Tracker) GetAllNodes() []*Node {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]*Node, 0, len(t.nodes))
	for _, node := range t.nodes {
		nodeCopy := *node
		if node.LatestReport != nil {
			reportCopy := *node.LatestReport
			nodeCopy.LatestReport = &reportCopy
		}
		result = append(result, &nodeCopy)
	}
	return result
}

// GetNodeName returns the name for a given UUID.
func (t *Tracker) GetNodeName(uuid string) string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.uuidToName[uuid]
}

// GetOnlineServers returns a list of formatted strings for online servers.
// Format: "- ServerName (uuid)" or "- uuid" if name is empty.
func (t *Tracker) GetOnlineServers() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var result []string
	for uuid := range t.onlineSet {
		name := t.uuidToName[uuid]
		if name != "" {
			result = append(result, fmt.Sprintf("- %s (%s)", name, uuid))
		} else {
			result = append(result, fmt.Sprintf("- %s", uuid))
		}
	}
	return result
}

// GetOfflineServers returns a list of formatted strings for offline servers.
func (t *Tracker) GetOfflineServers() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	offlineSet := make(map[string]bool)
	for uuid, node := range t.nodes {
		if !node.Online {
			offlineSet[uuid] = true
		}
	}
	var result []string
	for uuid := range offlineSet {
		name := t.uuidToName[uuid]
		if name != "" {
			result = append(result, fmt.Sprintf("- %s (%s)", name, uuid))
		} else {
			result = append(result, fmt.Sprintf("- %s", uuid))
		}
	}
	return result
}
