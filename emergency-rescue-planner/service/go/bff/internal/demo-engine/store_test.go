package demo_engine

import (
	"context"
	"testing"
)

// TestStore_AutoExpand_RoundTrip verifies the AutoExpand getter returns
// what was written via Upsert.
func TestStore_AutoExpand_RoundTrip(t *testing.T) {
	store := NewInMemoryDemoSessionStore()
	ctx := context.Background()

	if err := store.Upsert(ctx, "sess-1", 0.3, true); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	gotAutoExpand, active, err := store.GetSessionAutoExpand(ctx, "sess-1")
	if err != nil {
		t.Fatalf("get auto-expand: %v", err)
	}
	if !active {
		t.Errorf("expected session active=true after fresh upsert")
	}
	if !gotAutoExpand {
		t.Errorf("expected auto_expand=true, got false")
	}

	// Second heartbeat flips auto_expand off (e.g. user toggled it).
	if err := store.Upsert(ctx, "sess-1", 0.4, false); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}
	gotAutoExpand, active, _ = store.GetSessionAutoExpand(ctx, "sess-1")
	if !active {
		t.Errorf("expected session still active")
	}
	if gotAutoExpand {
		t.Errorf("expected auto_expand=false after toggle, got true")
	}

	// Unknown session is inactive, not an error.
	_, active, err = store.GetSessionAutoExpand(ctx, "sess-unknown")
	if err != nil {
		t.Errorf("unknown session: expected nil error, got %v", err)
	}
	if active {
		t.Errorf("expected unknown session to be inactive")
	}

	// Progress getter still works after the new field is wired in.
	progress, active, err := store.GetSessionProgress(ctx, "sess-1")
	if err != nil || !active {
		t.Errorf("progress getter regression: progress=%v active=%v err=%v", progress, active, err)
	}
	if progress != 0.4 {
		t.Errorf("expected progress=0.4, got %v", progress)
	}
}
