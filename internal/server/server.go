package server

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	connectionmanager "meme-fetcher/internal/connectionmanager"
	memeservice "meme-fetcher/internal/memeservice"
)

type Server struct {
	memeService       *memeservice.Service
	connectionManager *connectionmanager.Manager
	content           embed.FS
}

func NewServer(content embed.FS) *Server {
	return &Server{
		memeService:       memeservice.NewService(),
		connectionManager: connectionmanager.NewManager(50),
		content:           content,
	}
}

// SetupRoutes configures HTTP routes
func (s *Server) SetupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	// SSE endpoint
	mux.HandleFunc("/memes", s.handleMemeSSE)

	// Debug logs endpoint
	mux.HandleFunc("/debug", s.connectionManager.DebugHandler)

	// Client page with embedded template
	mux.HandleFunc("/", s.serveIndex)

	return mux
}

// handleMemeSSE manages Server-Sent Events for meme streaming
func (s *Server) handleMemeSSE(w http.ResponseWriter, r *http.Request) {
	// Register connection and get unique ID
	connID := s.connectionManager.AddConnection(r)
	s.connectionManager.AddConnectionEvent(connID, "Connection Established")

	// Log request details for debugging
	log.Printf("SSE Connection Received: %s %s (ID: %s)", r.Method, r.URL.Path, connID)
	log.Println("Request Headers:")
	for k, v := range r.Header {
		log.Printf("%s: %v", k, v)
		s.connectionManager.AddConnectionEvent(connID,
			fmt.Sprintf("Header: %s = %v", k, v))
	}

	// Ensure fresh meme data
	if err := s.memeService.FetchMemes(); err != nil {
		s.connectionManager.AddConnectionEvent(connID,
			fmt.Sprintf("Meme Fetch Error: %v", err))
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
		s.connectionManager.AddConnectionEvent(connID, "Streaming unsupported")
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
			s.connectionManager.AddConnectionEvent(connID, "Client connection closed")
			log.Printf("Connection %s closed", connID)
			return
		default:
			meme := s.memeService.GetRandomMeme()

			// Prepare SSE message
			message := fmt.Sprintf("data: {\"title\": %q, \"url\": %q, \"connID\": %q}\n\n",
				meme.Title, meme.URL, connID)

			// Write event
			_, err := fmt.Fprint(w, message)
			if err != nil {
				s.connectionManager.AddConnectionEvent(connID,
					fmt.Sprintf("Event Send Error: %v", err))
				log.Printf("Error sending event for %s: %v", connID, err)
				return
			}

			flusher.Flush()

			// Wait before next meme
			time.Sleep(5 * time.Second)
		}
	}
}

// serveIndex serves the embedded HTML template
func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFS(s.content, "web/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	tmpl.Execute(w, nil)
}
