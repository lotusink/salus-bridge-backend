package demo_engine

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// DemoEngine handles POST /api/demo/heartbeat.
type DemoEngine struct {
	store         DemoSessionStore
	heartbeatHook HeartbeatHookFn // feature 7; nil until SetHeartbeatHook called
}

// NewDemoEngine creates a DemoEngine backed by the given store.
func NewDemoEngine(store DemoSessionStore) *DemoEngine {
	return &DemoEngine{store: store}
}

// Store returns the underlying DemoSessionStore so feature 7 can call
// GetSessionProgress without going through an HTTP layer.
func (e *DemoEngine) Store() DemoSessionStore {
	return e.store
}

// SetHeartbeatHook registers fn to be called after each successful heartbeat write.
func (e *DemoEngine) SetHeartbeatHook(fn HeartbeatHookFn) {
	e.heartbeatHook = fn
}

// Heartbeat handles POST /api/demo/heartbeat.
// Required header: X-Volunteer-Session: <UUID>
// Body: { "progress": 0.0–1.0 }
// Response: 204 No Content
func (e *DemoEngine) Heartbeat(w http.ResponseWriter, r *http.Request) {
	session := r.Header.Get("X-Volunteer-Session")
	if session == "" {
		http.Error(w, "X-Volunteer-Session header is required", http.StatusBadRequest)
		return
	}

	var req HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err := req.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if err := e.store.Upsert(r.Context(), session, req.Progress, req.AutoExpand); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if e.heartbeatHook != nil {
		go e.heartbeatHook(session, req.Progress)
	}

	w.WriteHeader(http.StatusNoContent)
}
