package memeservice

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"
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

// Service manages meme retrieval and distribution
type Service struct {
	memes     []Meme
	mu        sync.RWMutex
	lastFetch time.Time
}

// NewService creates a new meme service
func NewService() *Service {
	return &Service{
		memes: []Meme{},
	}
}

// FetchMemes retrieves top memes from Reddit
func (ms *Service) FetchMemes() error {
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
func (ms *Service) GetRandomMeme() Meme {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	if len(ms.memes) == 0 {
		return Meme{Title: "No memes available", URL: ""}
	}

	return ms.memes[rand.Intn(len(ms.memes))]
}
