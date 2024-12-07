package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/rs/cors"
	"github.com/urfave/cli/v2"
	"golang.ngrok.com/ngrok"
	"golang.ngrok.com/ngrok/config"
)

// Meme represents the structure of a meme from Reddit
type Meme struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// RedditResponse represents the JSON response from Reddit
type RedditResponse struct {
	Data struct {
		Children []struct {
			Data Meme `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

// MemeService manages meme retrieval and distribution
type MemeService struct {
	memes     []Meme
	mu        sync.RWMutex
	lastFetch time.Time
}

// NewMemeService creates a new meme service
func NewMemeService() *MemeService {
	return &MemeService{
		memes: []Meme{},
	}
}

// FetchMemes retrieves top memes from Reddit
func (ms *MemeService) FetchMemes() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	// Limit fetch frequency
	if time.Since(ms.lastFetch) < 5*time.Minute {
		return nil
	}

	url := "https://www.reddit.com/r/memes.json?limit=26"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	// Set User-Agent to prevent Reddit from blocking
	req.Header.Set("User-Agent", "MemeSSEDebugger/1.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch memes: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %v", err)
	}

	var redditResp RedditResponse
	if err := json.Unmarshal(body, &redditResp); err != nil {
		return fmt.Errorf("failed to parse JSON: %v", err)
	}

	// Extract memes
	ms.memes = make([]Meme, 0, len(redditResp.Data.Children))
	for _, child := range redditResp.Data.Children {
		ms.memes = append(ms.memes, child.Data)
	}

	ms.lastFetch = time.Now()
	return nil
}

// GetRandomMeme returns a random meme
func (ms *MemeService) GetRandomMeme() Meme {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	if len(ms.memes) == 0 {
		return Meme{Title: "No memes available", URL: ""}
	}

	return ms.memes[rand.Intn(len(ms.memes))]
}

// ConnectionLog represents a detailed log of a single connection
type ConnectionLog struct {
	ID             string      `json:"id"`
	Timestamp      time.Time   `json:"timestamp"`
	RemoteAddr     string      `json:"remote_addr"`
	RequestHeaders http.Header `json:"request_headers"`
	Events         []string    `json:"events"`
}

// ConnectionManager handles multiple SSE connections and their logs
type ConnectionManager struct {
	mu             sync.RWMutex
	connections    map[string]*ConnectionLog
	maxConnections int
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(maxConnections int) *ConnectionManager {
	return &ConnectionManager{
		connections:    make(map[string]*ConnectionLog),
		maxConnections: maxConnections,
	}
}

// AddConnection registers a new connection and returns its ID
func (cm *ConnectionManager) AddConnection(r *http.Request) string {
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
func (cm *ConnectionManager) AddConnectionEvent(connID, event string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if conn, exists := cm.connections[connID]; exists {
		conn.Events = append(conn.Events, event)
	}
}

// GetConnectionLogs retrieves all connection logs
func (cm *ConnectionManager) GetConnectionLogs() []*ConnectionLog {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	logs := make([]*ConnectionLog, 0, len(cm.connections))
	for _, log := range cm.connections {
		logs = append(logs, log)
	}
	return logs
}

// DebugHandler provides an endpoint to retrieve connection logs
func (cm *ConnectionManager) DebugHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	logs := cm.GetConnectionLogs()

	if err := json.NewEncoder(w).Encode(logs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// MemeSSEHandler manages Server-Sent Events for meme streaming
func (ms *MemeService) MemeSSEHandler(cm *ConnectionManager, w http.ResponseWriter, r *http.Request) {
	// Register connection and get unique ID
	connID := cm.AddConnection(r)
	cm.AddConnectionEvent(connID, "Connection Established")

	// Log request details for debugging
	log.Printf("SSE Connection Received: %s %s (ID: %s)", r.Method, r.URL.Path, connID)
	log.Println("Request Headers:")
	for k, v := range r.Header {
		log.Printf("%s: %v", k, v)
		cm.AddConnectionEvent(connID, fmt.Sprintf("Header: %s = %v", k, v))
	}

	// Ensure fresh meme data
	if err := ms.FetchMemes(); err != nil {
		cm.AddConnectionEvent(connID, fmt.Sprintf("Meme Fetch Error: %v", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Flush headers
	flusher, ok := w.(http.Flusher)
	if !ok {
		cm.AddConnectionEvent(connID, "Streaming unsupported")
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}
	flusher.Flush()

	// Create channel for closing connection
	closeChan := r.Context().Done()

	// Meme streaming loop
	for {
		select {
		case <-closeChan:
			cm.AddConnectionEvent(connID, "Client connection closed")
			log.Printf("Connection %s closed", connID)
			return
		default:
			meme := ms.GetRandomMeme()

			// Prepare SSE message
			message := fmt.Sprintf("data: {\"title\": %q, \"url\": %q, \"connID\": %q}\n\n",
				meme.Title, meme.URL, connID)

			// Write event
			_, err := fmt.Fprint(w, message)
			if err != nil {
				cm.AddConnectionEvent(connID, fmt.Sprintf("Event Send Error: %v", err))
				log.Printf("Error sending event for %s: %v", connID, err)
				return
			}

			flusher.Flush()

			// Wait before next meme
			time.Sleep(5 * time.Second)
		}
	}
}

//go:embed templates/*
var content embed.FS

func main() {
	app := &cli.App{
		Name:  "meme-sse-debugger",
		Usage: "Server-Sent Events Meme Debugger with Ngrok Tunneling",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "port",
				Value: 8080,
				Usage: "Local server port",
			},
			&cli.BoolFlag{
				Name:  "tunnel",
				Usage: "Enable Ngrok tunneling",
			},
		},
		Action: func(ctx *cli.Context) error {
			// Seed random number generator
			rand.Seed(time.Now().UnixNano())

			// Create meme service and connection manager
			memeService := NewMemeService()
			connectionManager := NewConnectionManager(50)

			// CORS middleware
			handler := cors.Default().Handler(http.DefaultServeMux)

			// SSE endpoint
			http.HandleFunc("/memes", func(w http.ResponseWriter, r *http.Request) {
				memeService.MemeSSEHandler(connectionManager, w, r)
			})

			// Debug logs endpoint
			http.HandleFunc("/debug", connectionManager.DebugHandler)

			// Client page with embedded template
			http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				tmpl, err := template.ParseFS(content, "templates/index.html")
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "text/html")
				tmpl.Execute(w, nil)
			})

			// Port configuration
			port := fmt.Sprintf(":%d", ctx.Int("port"))

			// Optional Ngrok tunneling
			if ctx.Bool("tunnel") {
				tun, err := ngrok.Listen(ctx.Context,
					config.HTTPEndpoint(),
					ngrok.WithAuthtokenFromEnv(),
				)
				if err != nil {
					return fmt.Errorf("ngrok listen failed: %v", err)
				}

				log.Printf("Tunnel available at: %s", tun.URL())
				return http.Serve(tun, handler)
			}

			// Standard local server
			log.Printf("Server starting on %s", port)
			return http.ListenAndServe(port, handler)
		},
	}

	// Run the CLI app
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
