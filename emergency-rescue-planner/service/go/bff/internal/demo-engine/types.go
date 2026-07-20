package demo_engine

import "time"

// SessionTTL is the inactivity window after which a demo session is considered
// inactive. GetSessionProgress returns active=false when the last heartbeat is
// older than this value.
const SessionTTL = 60 * time.Second

// DemoSession is the domain representation of a volunteer's demo mode state.
// No JSON tags — internal store only; wire serialisation is on HeartbeatRequest.
type DemoSession struct {
	Progress          float64
	LastHeartbeat     time.Time
	AutoExpandEnabled bool // mirrors frontend "Auto-expand" toggle; read by route-anchor-engine β anchor fire path
}

// HeartbeatRequest is the wire type decoded from POST /api/demo/heartbeat body.
// AutoExpand is optional (zero-value false) so older clients without the field
// continue to work; new frontend always sends it explicitly.
type HeartbeatRequest struct {
	Progress   float64 `json:"progress"`
	AutoExpand bool    `json:"auto_expand,omitempty"`
}

// HeartbeatHookFn is called after each successful heartbeat store write to
// drive event-driven anchor spawning. The hook runs in a new goroutine so
// callers do not block on it.
type HeartbeatHookFn func(sessionID string, progress float64)
