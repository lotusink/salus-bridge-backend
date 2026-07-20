package demo_engine

import (
	"context"
	"sync"
	"time"
)

// DemoSessionStore stores per-volunteer demo mode progress with TTL.
// All implementations must be safe for concurrent use from multiple goroutines.
//
// Return semantics of GetSessionProgress / GetSessionAutoExpand:
//   - active=true:  session exists and last heartbeat is within SessionTTL; value is valid.
//   - active=false: session unknown OR last heartbeat older than SessionTTL; value is zero.
//   - non-nil error: infrastructure failure only (never returned for TTL expiry or missing session).
type DemoSessionStore interface {
	// Upsert writes progress + auto-expand state for sessionID and records
	// time.Now() as lastHeartbeat. A subsequent Upsert refreshes the TTL.
	Upsert(ctx context.Context, sessionID string, progress float64, autoExpand bool) error

	// GetSessionProgress returns the stored progress and activity status.
	// See type-level godoc for return semantics.
	GetSessionProgress(ctx context.Context, sessionID string) (progress float64, active bool, err error)

	// GetSessionAutoExpand returns the stored auto-expand flag and activity
	// status. Consumed by route-anchor-engine β anchor fire path to decide
	// whether to launch the expansion goroutine.
	GetSessionAutoExpand(ctx context.Context, sessionID string) (autoExpand bool, active bool, err error)
}

// InMemoryDemoSessionStore is a DemoSessionStore backed by an in-process sync.RWMutex map.
type InMemoryDemoSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*DemoSession
}

func NewInMemoryDemoSessionStore() *InMemoryDemoSessionStore {
	return &InMemoryDemoSessionStore{
		sessions: make(map[string]*DemoSession),
	}
}

func (s *InMemoryDemoSessionStore) Upsert(ctx context.Context, sessionID string, progress float64, autoExpand bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = &DemoSession{
		Progress:          progress,
		LastHeartbeat:     time.Now(),
		AutoExpandEnabled: autoExpand,
	}
	return nil
}

func (s *InMemoryDemoSessionStore) GetSessionProgress(ctx context.Context, sessionID string) (float64, bool, error) {
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		return 0, false, nil
	}
	if time.Since(sess.LastHeartbeat) >= SessionTTL {
		return 0, false, nil
	}
	return sess.Progress, true, nil
}

func (s *InMemoryDemoSessionStore) GetSessionAutoExpand(ctx context.Context, sessionID string) (bool, bool, error) {
	if err := ctx.Err(); err != nil {
		return false, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		return false, false, nil
	}
	if time.Since(sess.LastHeartbeat) >= SessionTTL {
		return false, false, nil
	}
	return sess.AutoExpandEnabled, true, nil
}
