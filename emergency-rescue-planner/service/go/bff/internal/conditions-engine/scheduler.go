package conditions_engine

import (
	"context"
	"time"
)

// ScheduleStep describes a single zone mutation in the scheduler's script.
type ScheduleStep struct {
	OffsetSeconds int    // seconds after cycle start when this step fires
	Action        string // "activate" | "update" | "clear"
	Zone          Zone   // for activate/update: full zone; for clear: only ID is used
}

// scheduleCycleLength is the total duration of one script cycle in seconds.
// A new cycle begins after this many seconds have elapsed since the cycle start.
const scheduleCycleLength = 95

// scheduleScript is the production zone mutation sequence committed as Go source.
// Each ~90 s cycle is guaranteed to broadcast at least one hazard_activated,
// one hazard_updated, and one hazard_cleared event.
//
// Note: rz2 is upgraded to High at offset 60 and remains High after the cycle resets.
// This is intentional — the scheduler demonstrates the activation lifecycle; a full
// restore-to-seed step is not part of v1 scope.
var scheduleScript = []ScheduleStep{
	{OffsetSeconds: 0, Action: "activate", Zone: zoneRZ1},
	{OffsetSeconds: 20, Action: "activate", Zone: zoneRZ4},
	{OffsetSeconds: 40, Action: "update", Zone: withLevel(zoneRZ1, ZoneLevelActive)},
	{OffsetSeconds: 60, Action: "update", Zone: withLevel(zoneRZ2, ZoneLevelHigh)},
	{OffsetSeconds: 75, Action: "clear", Zone: Zone{ID: "rz4"}},
	{OffsetSeconds: 90, Action: "clear", Zone: Zone{ID: "rz1"}},
}

// withLevel returns a copy of z with Level set and label / ActiveAlert adjusted.
// For ZoneLevelActive, the label is prefixed with "ACTIVE: ".
func withLevel(z Zone, level ZoneLevel) Zone {
	z.Level = level
	z.ActiveAlert = (level == ZoneLevelHigh || level == ZoneLevelActive)
	if level == ZoneLevelActive {
		z.Label = "ACTIVE: " + z.Label
	}
	return z
}

// Scheduler drives periodic zone mutations by cycling through scheduleScript,
// calling repo.UpsertZone or repo.DeleteZone, then broadcasting via hub.
type Scheduler struct {
	repo ZoneRepository
	hub  *Hub
	hook HazardActivatedFn // optional; fires D5 reroute proposals when set
}

// NewScheduler creates a Scheduler wired to the given repository and hub.
func NewScheduler(repo ZoneRepository, hub *Hub) *Scheduler {
	return &Scheduler{repo: repo, hub: hub}
}

func (s *Scheduler) setHook(fn HazardActivatedFn) { s.hook = fn }

// Start runs the scheduler loop until ctx is cancelled. Designed to be called
// in a goroutine; returns cleanly when ctx.Done() fires.
func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	cycleStart := time.Now()
	nextStep := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			elapsed := int(time.Since(cycleStart).Seconds())
			for nextStep < len(scheduleScript) && scheduleScript[nextStep].OffsetSeconds <= elapsed {
				s.fireStep(ctx, scheduleScript[nextStep])
				nextStep++
			}
			if nextStep >= len(scheduleScript) && elapsed >= scheduleCycleLength {
				cycleStart = time.Now()
				nextStep = 0
			}
		}
	}
}

func (s *Scheduler) fireStep(ctx context.Context, step ScheduleStep) {
	switch step.Action {
	case "activate":
		stored, err := s.repo.UpsertZone(ctx, step.Zone)
		if err != nil {
			return
		}
		s.hub.Broadcast(makeActivatedEnvelope(stored))
		if s.hook != nil {
			s.hook(ctx, stored)
		}
	case "update":
		stored, err := s.repo.UpsertZone(ctx, step.Zone)
		if err != nil {
			return
		}
		s.hub.Broadcast(makeUpdatedEnvelope(stored))
	case "clear":
		if err := s.repo.DeleteZone(ctx, step.Zone.ID); err != nil {
			return
		}
		s.hub.Broadcast(makeClearedEnvelope(step.Zone.ID))
	}
}
