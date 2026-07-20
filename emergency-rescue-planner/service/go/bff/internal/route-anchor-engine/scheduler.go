package route_anchor_engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	active_route_engine "bff/internal/active-route-engine"
	conditions_engine "bff/internal/conditions-engine"
	demo_engine "bff/internal/demo-engine"
	persons_engine "bff/internal/persons-engine"
)

type RouteScheduler struct {
	mu        sync.Mutex
	sessions  map[string]*RouteSchedulerSession
	triggered map[string]map[string]bool // sessionID → anchorID → true

	hazardScript []HazardAnchor
	personScript []PersonAnchor

	ctx        context.Context
	zoneRepo   conditions_engine.ZoneRepository
	cEngine    *conditions_engine.ConditionsEngine
	hub        *conditions_engine.Hub
	arRepo     active_route_engine.ActiveRouteRepository
	demoStore  demo_engine.DemoSessionStore
	personRepo persons_engine.PersonRepository

	// routeRecomputeHook is invoked by the β expansion goroutine when a
	// growing zone first intersects an active route. Wired in main.go to
	// active_route_engine.OnHazardActivated (re-uses the full D5 path:
	// origin-in-polygon check + ORS recompute + reroute_proposal emit).
	routeRecomputeHook conditions_engine.HazardActivatedFn
}

func newRouteScheduler(
	hazardScript []HazardAnchor,
	personScript []PersonAnchor,
	ctx context.Context,
	zoneRepo conditions_engine.ZoneRepository,
	cEngine *conditions_engine.ConditionsEngine,
	hub *conditions_engine.Hub,
	arRepo active_route_engine.ActiveRouteRepository,
	personRepo persons_engine.PersonRepository,
	demoStore demo_engine.DemoSessionStore,
) *RouteScheduler {
	return &RouteScheduler{
		sessions:     make(map[string]*RouteSchedulerSession),
		triggered:    make(map[string]map[string]bool),
		hazardScript: hazardScript,
		personScript: personScript,
		ctx:          ctx,
		zoneRepo:     zoneRepo,
		cEngine:      cEngine,
		hub:          hub,
		arRepo:       arRepo,
		demoStore:    demoStore,
		personRepo:   personRepo,
	}
}

func (s *RouteScheduler) OnHeartbeat(sessionID string, progress float64) {
	s.mu.Lock()

	// 1. Ensure session exists; lazy-init from active route.
	if _, exists := s.sessions[sessionID]; !exists {
		route, err := s.arRepo.GetBySession(s.ctx, sessionID)
		if err != nil {
			s.mu.Unlock()
			return // no active route for this session; skip silently
		}
		polyline := make([]LatLng, 0, len(route.Geometry.Coordinates))
		for _, c := range route.Geometry.Coordinates {
			polyline = append(polyline, LatLng{Lat: c[1], Lng: c[0]}) // GeoJSON [lng,lat] → LatLng
		}
		s.sessions[sessionID] = &RouteSchedulerSession{
			SessionID:     sessionID,
			ActiveRouteID: route.ID,
			Polyline:      polyline,
		}
	}
	sess := s.sessions[sessionID]

	// 2. Ensure triggered map exists for session.
	if s.triggered[sessionID] == nil {
		s.triggered[sessionID] = make(map[string]bool)
	}

	// 3. Walk hazardScript from NextHazardIdx while trigger <= progress.
	for sess.NextHazardIdx < len(s.hazardScript) {
		anchor := s.hazardScript[sess.NextHazardIdx]
		if anchor.TriggerAtProgress > progress {
			break
		}
		sess.NextHazardIdx++

		// Idempotency: pre-mark BEFORE releasing lock.
		if s.triggered[sessionID][anchor.ID] {
			continue
		}
		s.triggered[sessionID][anchor.ID] = true

		// Capture locals for goroutine closure.
		anchorCopy := anchor
		sessCopy := *sess // shallow copy; Polyline slice is safe (immutable after init)
		s.mu.Unlock()

		// Project, build zone, activate (may invoke ORS via D5 hook — must NOT hold lock).
		centroid, dir := ProjectAnchorToLatLng(sessCopy.Polyline, anchorCopy.CentroidAtProgress)
		if anchorCopy.PerpendicularOffsetM != 0 {
			centroid = ApplyPerpendicularOffset(centroid, dir, anchorCopy.PerpendicularOffsetM)
		}
		poly := ApplyPolygonOffsets(centroid, anchorCopy.PolygonOffsets)
		zoneID := fmt.Sprintf("rha-%s-%s", sessionID, anchorCopy.ID)
		zone := conditions_engine.Zone{
			ID:     zoneID,
			Level:  anchorCopy.Level,
			Label:  anchorCopy.Label,
			Source: anchorCopy.Source,
			ActiveAlert: anchorCopy.Level == conditions_engine.ZoneLevelHigh ||
				anchorCopy.Level == conditions_engine.ZoneLevelActive,
			Polygon:   poly,
			UpdatedAt: time.Now(),
		}
		if err := s.cEngine.ActivateZone(s.ctx, zone); err == nil {
			s.mu.Lock()
			s.sessions[sessionID].SpawnedZoneIDs = append(s.sessions[sessionID].SpawnedZoneIDs, zoneID)
			// (do NOT re-mark triggered; already marked before Unlock)

			// δ lifecycle: schedule auto-deactivation if anchor opts in.
			if anchorCopy.LifecycleMs > 0 {
				delay := time.Duration(anchorCopy.LifecycleMs) * time.Millisecond
				go func(sid, zid string) {
					select {
					case <-time.After(delay):
						s.deleteSpawnedZone(sid, zid)
					case <-s.ctx.Done():
						return
					}
				}(sessionID, zoneID)
			}

			// β expansion: launch goroutine if anchor opts in AND session has
			// auto_expand on at this moment. Read happens under demoStore's
			// own lock; the goroutine then re-reads through ctx on each step.
			if anchorCopy.Behavior == "expand" {
				if autoExpand, _, _ := s.demoStore.GetSessionAutoExpand(s.ctx, sessionID); autoExpand {
					go s.runExpansion(sessionID, zoneID, anchorCopy, centroid)
				}
			}
		} else {
			s.mu.Lock()
			// ActivateZone failed; triggered is already marked — anchor will not retry.
		}
	}

	// 4. Walk personScript from NextPersonIdx while trigger <= progress.
	for sess.NextPersonIdx < len(s.personScript) {
		anchor := s.personScript[sess.NextPersonIdx]
		if anchor.TriggerAtProgress > progress {
			break
		}
		sess.NextPersonIdx++

		if s.triggered[sessionID][anchor.ID] {
			continue
		}
		s.triggered[sessionID][anchor.ID] = true

		anchorCopy := anchor
		sessCopy := *sess
		s.mu.Unlock()

		centroid, dir := ProjectAnchorToLatLng(sessCopy.Polyline, anchorCopy.CentroidAtProgress)
		if anchorCopy.PerpendicularOffsetM != 0 {
			centroid = ApplyPerpendicularOffset(centroid, dir, anchorCopy.PerpendicularOffsetM)
		}
		personID := fmt.Sprintf("rpa-%s-%s", sessionID, anchorCopy.ID)
		tmpl := anchorCopy.PersonTemplate
		needs := tmpl.Needs
		if needs == nil {
			needs = make([]string, 0)
		}
		dest := persons_engine.PersonDestination{
			Lat: centroid.Lat, Lng: centroid.Lng,
			Label: tmpl.DestinationLabel,
		}
		person := persons_engine.Person{
			ID: personID, Label: tmpl.Label,
			Needs: needs, NeedsSummary: tmpl.NeedsSummary,
			CtaLabel: tmpl.CtaLabel, SupportGuideID: tmpl.SupportGuideID,
			Lat: centroid.Lat, Lng: centroid.Lng,
			Destination: dest,
		}
		storedPerson, err := s.personRepo.UpsertPerson(s.ctx, person)
		if err == nil {
			wire := personToWire(storedPerson)
			envelope := conditions_engine.WsEnvelopeWire{
				Type: "person_added",
				Ts:   time.Now().UTC().Format(time.RFC3339),
				Payload: PersonAddedPayloadWire{
					Person: wire, ActiveRouteID: sessCopy.ActiveRouteID,
				},
			}
			_ = s.hub.SendToClient(sessionID, envelope)
			s.mu.Lock()
			s.sessions[sessionID].SpawnedPersonIDs = append(s.sessions[sessionID].SpawnedPersonIDs, personID)
		} else {
			s.mu.Lock()
		}
	}

	s.mu.Unlock()
}

// OnRerouteAccepted updates a session's polyline and resets its script walk
// indices after the volunteer accepts a reroute proposal. Already-triggered
// anchors stay in the triggered map (they must not refire on the new route);
// untriggered anchors get a fresh chance to fire as user progress climbs on
// the new polyline. No-op when the session is unknown (e.g. AcceptReroute
// before the first heartbeat).
func (s *RouteScheduler) OnRerouteAccepted(ctx context.Context, sessionID string, newPolyline []LatLng) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, exists := s.sessions[sessionID]
	if !exists {
		return
	}
	sess.Polyline = newPolyline
	sess.NextHazardIdx = 0
	sess.NextPersonIdx = 0
}

// runExpansion is the β-anchor goroutine. Grows a spawned zone's polygon by
// linearly increasing the scale factor from 1.0 to ExpansionTargetFactor over
// a sequence of ExpansionStepMs-spaced ticks. Each tick:
//  1. Compute new polygon by scaling PolygonOffsets relative to centroid.
//  2. Call cEngine.UpdateZone — broadcasts hazard_updated (no D5).
//  3. Check intersection vs. session's active route. On intersect, invoke
//     routeRecomputeHook (D5 reroute) exactly once and exit.
//
// Exit conditions:
//   - intersect with active route → routeRecomputeHook fires → return
//   - factor reaches ExpansionTargetFactor without intersect → return
//   - ctx.Done() → return (clean shutdown, no leak)
//   - zone no longer exists in repo (deleted by OnRouteDeleted) → return
//   - active route no longer exists in arRepo → return
func (s *RouteScheduler) runExpansion(sessionID, zoneID string, anchor HazardAnchor, centroid LatLng) {
	target := anchor.ExpansionTargetFactor
	if target <= 1 {
		return // misconfig — nothing to expand
	}
	stepMs := anchor.ExpansionStepMs
	if stepMs <= 0 {
		stepMs = 2000
	}

	// 10 evenly-spaced increments from 1 → target.
	const steps = 10
	factorStep := (target - 1.0) / steps
	factor := 1.0

	ticker := time.NewTicker(time.Duration(stepMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
		case <-s.ctx.Done():
			return
		}

		// Resurrection guard: if OnRouteDeleted (or session TTL cleanup) has
		// removed the session or the spawned zone, do not re-UpsertZone via
		// UpdateZone — otherwise the broadcast would resurrect a dead zone.
		s.mu.Lock()
		sess, sessAlive := s.sessions[sessionID]
		stillSpawned := false
		if sessAlive {
			for _, z := range sess.SpawnedZoneIDs {
				if z == zoneID {
					stillSpawned = true
					break
				}
			}
		}
		s.mu.Unlock()
		if !sessAlive || !stillSpawned {
			return
		}

		factor += factorStep
		if factor > target {
			factor = target
		}

		// Build scaled polygon.
		scaled := make([]OffsetM, len(anchor.PolygonOffsets))
		for i, off := range anchor.PolygonOffsets {
			scaled[i] = OffsetM{NorthM: off.NorthM * factor, EastM: off.EastM * factor}
		}
		newPoly := ApplyPolygonOffsets(centroid, scaled)

		updatedZone := conditions_engine.Zone{
			ID:     zoneID,
			Level:  anchor.Level,
			Label:  anchor.Label,
			Source: anchor.Source,
			ActiveAlert: anchor.Level == conditions_engine.ZoneLevelHigh ||
				anchor.Level == conditions_engine.ZoneLevelActive,
			Polygon:   newPoly,
			UpdatedAt: time.Now(),
		}

		// Broadcast update (no D5 here — that's only on intersect).
		if err := s.cEngine.UpdateZone(s.ctx, updatedZone); err != nil {
			return // zone likely deleted; give up
		}

		// Fetch the session's current active route. If gone, stop.
		route, err := s.arRepo.GetBySession(s.ctx, sessionID)
		if err != nil {
			return
		}

		if active_route_engine.PolylineIntersectsPolygon(route.Geometry.Coordinates, newPoly) {
			// First intersect — fire D5 once and stop expansion.
			if s.routeRecomputeHook != nil {
				s.routeRecomputeHook(s.ctx, updatedZone)
			}
			return
		}

		if factor >= target {
			return // reached max without intersect
		}
	}
}

// deleteSpawnedZone removes a single spawned zone: drops it from the session's
// SpawnedZoneIDs slice, calls zoneRepo.DeleteZone, and broadcasts hazard_cleared.
// Used by both the δ lifecycle goroutine and (indirectly) OnRouteDeleted.
// Safe to call concurrently; takes its own lock.
func (s *RouteScheduler) deleteSpawnedZone(sessionID, zoneID string) {
	s.mu.Lock()
	if sess, ok := s.sessions[sessionID]; ok {
		newIDs := make([]string, 0, len(sess.SpawnedZoneIDs))
		for _, z := range sess.SpawnedZoneIDs {
			if z != zoneID {
				newIDs = append(newIDs, z)
			}
		}
		sess.SpawnedZoneIDs = newIDs
	}
	s.mu.Unlock()

	_ = s.zoneRepo.DeleteZone(s.ctx, zoneID)
	s.hub.Broadcast(makeLocalClearedEnvelope(zoneID))
}

func (s *RouteScheduler) OnRouteDeleted(sessionID string) {
	s.mu.Lock()
	sess := s.sessions[sessionID]
	if sess == nil {
		s.mu.Unlock()
		return
	}
	spawnedZones := append([]string(nil), sess.SpawnedZoneIDs...)
	spawnedPersons := append([]string(nil), sess.SpawnedPersonIDs...)
	spawnedOriginAnchors := append([]string(nil), sess.SpawnedOriginAnchorIDs...)
	delete(s.sessions, sessionID)
	delete(s.triggered, sessionID)
	s.mu.Unlock()

	for _, zid := range spawnedZones {
		_ = s.zoneRepo.DeleteZone(s.ctx, zid)
		s.hub.Broadcast(makeLocalClearedEnvelope(zid))
	}
	for _, zid := range spawnedOriginAnchors {
		_ = s.zoneRepo.DeleteZone(s.ctx, zid)
		s.hub.Broadcast(makeLocalClearedEnvelope(zid))
	}
	for _, pid := range spawnedPersons {
		_ = s.personRepo.DeletePerson(s.ctx, pid)
		envelope := conditions_engine.WsEnvelopeWire{
			Type:    "person_removed",
			Ts:      time.Now().UTC().Format(time.RFC3339),
			Payload: PersonRemovedPayloadWire{PersonID: pid},
		}
		_ = s.hub.SendToClient(sessionID, envelope)
	}
}

func (s *RouteScheduler) startTTLCleaner(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.mu.Lock()
				var expired []string
				for sid := range s.sessions {
					_, active, _ := s.demoStore.GetSessionProgress(ctx, sid)
					if !active {
						expired = append(expired, sid)
					}
				}
				s.mu.Unlock()
				for _, sid := range expired {
					s.OnRouteDeleted(sid)
				}
			}
		}
	}()
}

// personToWire converts a persons_engine.Person to PersonWire for WS payloads.
func personToWire(p persons_engine.Person) persons_engine.PersonWire {
	needs := p.Needs
	if needs == nil {
		needs = make([]string, 0)
	}
	return persons_engine.PersonWire{
		ID: p.ID, Lat: p.Lat, Lng: p.Lng, Label: p.Label,
		Needs: needs, NeedsSummary: p.NeedsSummary,
		CtaLabel: p.CtaLabel, SupportGuideID: p.SupportGuideID,
		Destination: persons_engine.PersonDestinationWire{
			Lat: p.Destination.Lat, Lng: p.Destination.Lng, Label: p.Destination.Label,
		},
	}
}

// makeLocalClearedEnvelope builds a hazard_cleared WsEnvelopeWire using exported conditions_engine types.
func makeLocalClearedEnvelope(zoneID string) conditions_engine.WsEnvelopeWire {
	return conditions_engine.WsEnvelopeWire{
		Type:    "hazard_cleared",
		Ts:      time.Now().UTC().Format(time.RFC3339),
		Payload: conditions_engine.HazardClearedPayloadWire{ZoneID: zoneID},
	}
}
