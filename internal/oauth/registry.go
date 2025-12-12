package oauth

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/nghyane/llm-mux/internal/misc"
)

// RequestMode indicates how the OAuth flow was initiated.
type RequestMode string

const (
	// ModeCLI indicates the request was initiated from CLI (blocking wait).
	ModeCLI RequestMode = "cli"
	// ModeWebUI indicates the request was initiated from Web UI (async with postMessage).
	ModeWebUI RequestMode = "webui"
)

// RequestStatus tracks the current state of an OAuth request.
type RequestStatus string

const (
	StatusPending   RequestStatus = "pending"
	StatusCompleted RequestStatus = "completed"
	StatusFailed    RequestStatus = "failed"
	StatusExpired   RequestStatus = "expired"
	StatusCancelled RequestStatus = "cancelled"
)

// OAuthResult contains the result of an OAuth callback.
type OAuthResult struct {
	Code  string // Authorization code from OAuth provider
	State string // State parameter for validation
	Error string // Error message if authentication failed
}

// OAuthRequest represents a pending OAuth authentication request.
type OAuthRequest struct {
	ID        string        // Unique identifier for this request
	State     string        // OAuth state parameter (used as key)
	Provider  string        // Provider name (claude, gemini, codex, etc.)
	Mode      RequestMode   // CLI or WebUI
	Status    RequestStatus // Current status
	Error     string        // Error message if failed
	AuthURL   string        // Full authorization URL to open in browser
	CreatedAt time.Time     // When the request was created
	ExpiresAt time.Time     // When the request expires (TTL)

	// ResultChan receives the OAuth callback result.
	// CLI mode blocks on this channel; WebUI mode uses it for internal signaling.
	ResultChan    chan *OAuthResult
	channelClosed atomic.Bool // Prevents double channel close

	// PKCE fields (if applicable)
	CodeVerifier  string
	CodeChallenge string

	// Additional metadata
	RedirectURI string
	Scopes      []string
}

// Registry manages pending OAuth requests with thread-safe access.
type Registry struct {
	mu       sync.RWMutex
	requests map[string]*OAuthRequest // keyed by state
	byID     map[string]*OAuthRequest // secondary index by ID

	// Configuration
	defaultTTL time.Duration
}

// NewRegistry creates a new OAuth request registry.
func NewRegistry() *Registry {
	r := &Registry{
		requests:   make(map[string]*OAuthRequest),
		byID:       make(map[string]*OAuthRequest),
		defaultTTL: 5 * time.Minute,
	}
	// Start cleanup goroutine
	go r.cleanupLoop()
	return r
}

// Register creates and stores a new OAuth request.
func (r *Registry) Register(provider string, mode RequestMode) (*OAuthRequest, error) {
	state, err := misc.GenerateRandomState()
	if err != nil {
		return nil, err
	}

	id, err := misc.GenerateRandomState()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	req := &OAuthRequest{
		ID:         id,
		State:      state,
		Provider:   provider,
		Mode:       mode,
		Status:     StatusPending,
		CreatedAt:  now,
		ExpiresAt:  now.Add(r.defaultTTL),
		ResultChan: make(chan *OAuthResult, 1), // Buffered to prevent blocking
	}

	r.mu.Lock()
	r.requests[state] = req
	r.byID[id] = req
	r.mu.Unlock()

	return req, nil
}

// Get retrieves a request by state parameter.
func (r *Registry) Get(state string) *OAuthRequest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.requests[state]
}

// GetByID retrieves a request by its unique ID.
func (r *Registry) GetByID(id string) *OAuthRequest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byID[id]
}

// Complete marks a request as completed with the given result.
// Holds lock through channel send to prevent TOCTOU race with Remove().
func (r *Registry) Complete(state string, result *OAuthResult) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	req, exists := r.requests[state]
	if !exists || req.Status != StatusPending {
		return false
	}
	req.Status = StatusCompleted

	// Send result to channel while holding lock (non-blocking due to buffer)
	select {
	case req.ResultChan <- result:
	default:
		// Channel already has a result, ignore
	}

	return true
}

// Fail marks a request as failed with an error message.
// Holds lock through channel send to prevent TOCTOU race with Remove().
func (r *Registry) Fail(state string, errMsg string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	req, exists := r.requests[state]
	if !exists {
		// Create a new request if it doesn't exist (for backward compatibility)
		req = &OAuthRequest{
			ID:         state,
			State:      state,
			Status:     StatusFailed,
			Error:      errMsg,
			ResultChan: make(chan *OAuthResult, 1),
		}
		r.requests[state] = req
		r.byID[state] = req
		return true
	}
	req.Status = StatusFailed
	req.Error = errMsg

	// Send error result to channel while holding lock
	select {
	case req.ResultChan <- &OAuthResult{State: state, Error: errMsg}:
	default:
	}

	return true
}

// Cancel cancels a pending request.
// Holds lock through channel send to prevent TOCTOU race with Remove().
func (r *Registry) Cancel(state string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	req, exists := r.requests[state]
	if !exists || req.Status != StatusPending {
		return false
	}
	req.Status = StatusCancelled
	req.Error = "cancelled"

	// Send cancellation to channel while holding lock
	select {
	case req.ResultChan <- &OAuthResult{State: state, Error: "cancelled"}:
	default:
	}

	return true
}

// Remove deletes a request from the registry.
func (r *Registry) Remove(state string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	req, exists := r.requests[state]
	if !exists {
		return
	}

	delete(r.requests, state)
	delete(r.byID, req.ID)

	// Safe channel close using atomic flag to prevent double close panic
	if req.channelClosed.CompareAndSwap(false, true) {
		close(req.ResultChan)
	}
}

// GetStatus returns the current status of a request.
func (r *Registry) GetStatus(state string) (RequestStatus, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	req, exists := r.requests[state]
	if !exists {
		return "", false
	}
	return req.Status, true
}

// cleanupLoop periodically removes expired requests.
func (r *Registry) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		r.cleanup()
	}
}

// cleanup removes expired requests.
// Uses single write lock to prevent race conditions.
func (r *Registry) cleanup() {
	now := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()

	for state, req := range r.requests {
		if !now.After(req.ExpiresAt) {
			continue
		}

		// Only expire pending requests
		if req.Status == StatusPending {
			req.Status = StatusExpired
			req.Error = "expired"
			// Send expiry notification (non-blocking)
			select {
			case req.ResultChan <- &OAuthResult{State: state, Error: "expired"}:
			default:
			}
		}

		// Remove from maps
		delete(r.requests, state)
		delete(r.byID, req.ID)
	}
}

// Create creates a new OAuth request with a given state (for backward compatibility).
// Unlike Register(), this allows specifying the state parameter directly.
func (r *Registry) Create(state, provider string, mode RequestMode) *OAuthRequest {
	now := time.Now()
	id := state // Use state as ID for simplicity

	req := &OAuthRequest{
		ID:         id,
		State:      state,
		Provider:   provider,
		Mode:       mode,
		Status:     StatusPending,
		CreatedAt:  now,
		ExpiresAt:  now.Add(r.defaultTTL),
		ResultChan: make(chan *OAuthResult, 1),
	}

	r.mu.Lock()
	r.requests[state] = req
	r.byID[id] = req
	r.mu.Unlock()

	return req
}

// Delete removes a request from the registry (alias for Remove for backward compatibility).
func (r *Registry) Delete(state string) {
	r.Remove(state)
}

// Stats returns statistics about the registry.
func (r *Registry) Stats() map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := map[string]int{
		"total":     len(r.requests),
		"pending":   0,
		"completed": 0,
		"failed":    0,
	}

	for _, req := range r.requests {
		switch req.Status {
		case StatusPending:
			stats["pending"]++
		case StatusCompleted:
			stats["completed"]++
		case StatusFailed, StatusExpired, StatusCancelled:
			stats["failed"]++
		}
	}

	return stats
}
