package connectionmanager

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ConnectionLog represents a detailed log of a single connection
type ConnectionLog struct {
	ID             string      `json:"id"`
	Timestamp      time.Time   `json:"timestamp"`
	RemoteAddr     string      `json:"remote_addr"`
	RequestHeaders http.Header `json:"request_headers"`
	Events         []string    `json:"events"`
}

// Manager handles multiple SSE connections and their logs
type Manager struct {
	mu             sync.RWMutex
	connections    map[string]*ConnectionLog
	maxConnections int
}

// NewManager creates a new connection manager
func NewManager(maxConnections int) *Manager {
	return &Manager{
		connections:    make(map[string]*ConnectionLog),
		maxConnections: maxConnections,
	}
}

// AddConnection registers a new connection and returns its ID
func (cm *Manager) AddConnection(r *http.Request) string {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Generate unique connection ID
	connID := fmt.Sprintf("conn_%d", len(cm.connections)+1)

	// Create connection log
	connLog := &ConnectionLog{
		ID:             connID,
		Timestamp:      time.Now(),
		RemoteAddr:     r.RemoteAddr,
		RequestHeaders: r.Header,
		Events:         []string{},
	}

	// Add log entry
	cm.connections[connID] = connLog

	// Trim connections if exceeding max
	if len(cm.connections) > cm.maxConnections {
		var oldestKey string
		var oldestTime time.Time

		for k, v := range cm.connections {
			if oldestKey == "" || v.Timestamp.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.Timestamp
			}
		}

		delete(cm.connections, oldestKey)
	}

	return connID
}

// AddConnectionEvent logs an event for a specific connection
func (cm *Manager) AddConnectionEvent(connID, event string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if conn, exists := cm.connections[connID]; exists {
		conn.Events = append(conn.Events, event)
	}
}

// GetConnectionLogs retrieves all connection logs
func (cm *Manager) GetConnectionLogs() []*ConnectionLog {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	logs := make([]*ConnectionLog, 0, len(cm.connections))
	for _, log := range cm.connections {
		logs = append(logs, log)
	}
	return logs
}

// DebugHandler provides an endpoint to retrieve connection logs
func (cm *Manager) DebugHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	logs := cm.GetConnectionLogs()

	if err := json.NewEncoder(w).Encode(logs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
