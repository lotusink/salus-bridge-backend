package conditions_engine

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ConditionsEngine handles GET /api/conditions/risk-zones and GET /ws/hazards.
type ConditionsEngine struct {
	repo        ZoneRepository
	hub         *Hub
	sched       *Scheduler
	upgrader    websocket.Upgrader
	hazardHook  HazardActivatedFn
	appCtx      context.Context // set by StartHub; used for scheduler goroutine lifetime
	schedMu     sync.Mutex
	schedCancel context.CancelFunc // non-nil when wall-clock scheduler is running
}

// NewConditionsEngine creates a ConditionsEngine and prepares the WebSocket upgrader.
// Zones are NOT seeded here; call StartHazardDemo (via HTTP or StartRealtime) to seed.
func NewConditionsEngine(repo ZoneRepository) *ConditionsEngine {
	return &ConditionsEngine{
		repo: repo,
		upgrader: websocket.Upgrader{
			Subprotocols: []string{"volunteerlink.hazards.v1"},
			CheckOrigin:  makeCheckOrigin(),
		},
	}
}

// GetRiskZones handles GET /api/conditions/risk-zones.
//
// @Summary      List risk zones near a coordinate
// @Description  Returns seeded Melbourne risk zones filtered by haversine distance from (lat, lng).
// @Tags         conditions
// @Produce      json
// @Param        lat    query number false "Origin latitude (-90 to 90)"
// @Param        lng    query number false "Origin longitude (-180 to 180)"
// @Param        radius query number false "Search radius in km (default 100)"
// @Success      200 {object} GetRiskZonesResponse
// @Failure      400 {string} string "invalid request"
// @Router       /api/conditions/risk-zones [get]
func (e *ConditionsEngine) GetRiskZones(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	latStr := q.Get("lat")
	lngStr := q.Get("lng")
	radiusStr := q.Get("radius")

	var zones []Zone
	var err error

	if latStr != "" || lngStr != "" {
		lat, err1 := strconv.ParseFloat(latStr, 64)
		lng, err2 := strconv.ParseFloat(lngStr, 64)
		if err1 != nil || err2 != nil {
			http.Error(w, "invalid lat or lng: must be numeric", http.StatusBadRequest)
			return
		}
		if lat < -90 || lat > 90 {
			http.Error(w, fmt.Sprintf("lat %v out of range: must be -90 to 90", lat), http.StatusBadRequest)
			return
		}
		if lng < -180 || lng > 180 {
			http.Error(w, fmt.Sprintf("lng %v out of range: must be -180 to 180", lng), http.StatusBadRequest)
			return
		}
		radius := 100.0
		if radiusStr != "" {
			radius, err = strconv.ParseFloat(radiusStr, 64)
			if err != nil || radius <= 0 {
				http.Error(w, "invalid radius: must be a positive number", http.StatusBadRequest)
				return
			}
		}
		zones, err = e.repo.GetZonesNear(r.Context(), lat, lng, radius)
	} else {
		zones, err = e.repo.GetAll(r.Context())
	}

	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	wires := make([]ZoneWire, 0, len(zones))
	for _, z := range zones {
		wires = append(wires, zoneToWire(z))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(GetRiskZonesResponse{
		Data: GetRiskZonesData{Zones: wires},
	}); err != nil {
		return
	}
}

// makeCheckOrigin returns a CheckOrigin func that allows only the frontend
// URL derived from the same env vars as cors.go. Evaluated once at engine init.
func makeCheckOrigin() func(r *http.Request) bool {
	var allowed string
	if os.Getenv("ENV") == "deployment" {
		allowed = os.Getenv("DEPLOYMENT_FRONTEND_URL")
	} else {
		allowed = os.Getenv("LOCAL_FRONTEND_URL")
	}
	return func(r *http.Request) bool {
		return r.Header.Get("Origin") == allowed
	}
}

// zoneToWire converts a Zone domain type to its JSON wire representation.
func zoneToWire(z Zone) ZoneWire {
	return ZoneWire{
		ID:          z.ID,
		Level:       string(z.Level),
		Label:       z.Label,
		Source:      z.Source,
		UpdatedAt:   z.UpdatedAt.UTC().Format(time.RFC3339),
		ActiveAlert: z.ActiveAlert,
		Polygon:     z.Polygon,
	}
}

// newClientID generates a unique string ID for a new WS connection.
func newClientID() string {
	return fmt.Sprintf("ws-client-%d-%d", time.Now().UnixNano(), rand.Int63n(100000))
}

// makeActivatedEnvelope creates a hazard_activated WsEnvelopeWire.
func makeActivatedEnvelope(z Zone) WsEnvelopeWire {
	return WsEnvelopeWire{
		Type:    "hazard_activated",
		Ts:      time.Now().UTC().Format(time.RFC3339),
		Payload: HazardActivatedPayloadWire{Zone: zoneToWire(z)},
	}
}

// makeUpdatedEnvelope creates a hazard_updated WsEnvelopeWire.
func makeUpdatedEnvelope(z Zone) WsEnvelopeWire {
	return WsEnvelopeWire{
		Type:    "hazard_updated",
		Ts:      time.Now().UTC().Format(time.RFC3339),
		Payload: HazardUpdatedPayloadWire{Zone: zoneToWire(z)},
	}
}

// makeClearedEnvelope creates a hazard_cleared WsEnvelopeWire.
func makeClearedEnvelope(zoneID string) WsEnvelopeWire {
	return WsEnvelopeWire{
		Type:    "hazard_cleared",
		Ts:      time.Now().UTC().Format(time.RFC3339),
		Payload: HazardClearedPayloadWire{ZoneID: zoneID},
	}
}

// StartHub creates the Hub and wires up CloseAll on ctx cancel.
// Must be called before StartHazardDemo or any HTTP handler.
func (e *ConditionsEngine) StartHub(ctx context.Context) {
	e.appCtx = ctx
	e.hub = NewHub()
	go func() {
		<-ctx.Done()
		e.hub.CloseAll()
	}()
}

// StartHazardDemo seeds the 5 wall-clock zones, creates a Scheduler, and starts it.
// Idempotent: calling while already active is a no-op.
// Must be called after StartHub (appCtx must be set).
func (e *ConditionsEngine) StartHazardDemo(ctx context.Context) error {
	e.schedMu.Lock()
	defer e.schedMu.Unlock()
	if e.schedCancel != nil {
		return nil // already running
	}
	seedZones(ctx, e.repo)
	schedCtx, cancel := context.WithCancel(e.appCtx)
	e.schedCancel = cancel
	e.sched = NewScheduler(e.repo, e.hub)
	if e.hazardHook != nil {
		e.sched.setHook(e.hazardHook)
	}
	go e.sched.Start(schedCtx)
	return nil
}

// StopHazardDemo cancels the wall-clock scheduler and removes all wall-clock-sourced zones.
// Broadcasts hazard_cleared for each removed zone. Idempotent.
func (e *ConditionsEngine) StopHazardDemo(ctx context.Context) error {
	e.schedMu.Lock()
	if e.schedCancel != nil {
		e.schedCancel()
		e.schedCancel = nil
		e.sched = nil
	}
	e.schedMu.Unlock()

	ids, err := e.repo.DeleteZonesBySource(ctx, "wall-clock")
	if err != nil {
		return err
	}
	for _, id := range ids {
		e.hub.Broadcast(makeClearedEnvelope(id))
	}
	return nil
}

// StartRealtime is a deprecated alias for StartHub + StartHazardDemo.
// Retained for compatibility. New code should call StartHub then use the HTTP endpoints.
func (e *ConditionsEngine) StartRealtime(ctx context.Context) {
	e.StartHub(ctx)
	_ = e.StartHazardDemo(ctx)
}

// Hub returns the Hub so external packages (e.g. active-route-engine) can call SendToClient.
// Returns nil before StartRealtime is called.
func (e *ConditionsEngine) Hub() *Hub { return e.hub }

// SetHazardHook registers fn to be called after every ActivateZone call and after
// every scheduler "activate" step. Must be called after StartRealtime.
func (e *ConditionsEngine) SetHazardHook(fn HazardActivatedFn) {
	e.hazardHook = fn
	if e.sched != nil {
		e.sched.setHook(fn)
	}
}

// StartHazardDemoHTTP handles POST /api/demo/hazard-demo/start.
// Starts the wall-clock hazard scheduler and seeds the 5 Melbourne zones.
// No request body required. No X-Volunteer-Session required.
// Response: 204 No Content.
//
// @Summary      Start wall-clock hazard demo
// @Tags         demo
// @Success      204
// @Failure      500 {string} string "internal error"
// @Router       /api/demo/hazard-demo/start [post]
func (e *ConditionsEngine) StartHazardDemoHTTP(w http.ResponseWriter, r *http.Request) {
	if err := e.StartHazardDemo(r.Context()); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// StopHazardDemoHTTP handles POST /api/demo/hazard-demo/stop.
// Stops the wall-clock hazard scheduler and clears all wall-clock zones.
// No request body required. No X-Volunteer-Session required.
// Response: 204 No Content.
//
// @Summary      Stop wall-clock hazard demo
// @Tags         demo
// @Success      204
// @Failure      500 {string} string "internal error"
// @Router       /api/demo/hazard-demo/stop [post]
func (e *ConditionsEngine) StopHazardDemoHTTP(w http.ResponseWriter, r *http.Request) {
	if err := e.StopHazardDemo(r.Context()); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ActivateZone upserts zone into the repository, broadcasts hazard_activated to all
// clients, and invokes the registered hazard hook (if any). Safe to call concurrently.
// Used by cluster-engine to promote field-report clusters to hazard zones.
func (e *ConditionsEngine) ActivateZone(ctx context.Context, zone Zone) error {
	stored, err := e.repo.UpsertZone(ctx, zone)
	if err != nil {
		return err
	}
	e.hub.Broadcast(makeActivatedEnvelope(stored))
	if e.hazardHook != nil {
		e.hazardHook(ctx, stored)
	}
	return nil
}

// UpdateZone upserts zone into the repository and broadcasts hazard_updated to
// all clients. Unlike ActivateZone it does NOT invoke the hazard hook — the
// "updated" semantics imply geometry / metadata change without re-triggering
// D5 from this entry point. Callers that need D5 re-check on an update (e.g.
// route-anchor-engine β expansion goroutine when intersect is first detected)
// must invoke the hook explicitly via their own dependency wiring.
func (e *ConditionsEngine) UpdateZone(ctx context.Context, zone Zone) error {
	stored, err := e.repo.UpsertZone(ctx, zone)
	if err != nil {
		return err
	}
	e.hub.Broadcast(makeUpdatedEnvelope(stored))
	return nil
}

// WsHazards handles GET /ws/hazards — upgrades an HTTP request to a WebSocket
// connection and registers it with the Hub.
//
// @Summary      WebSocket hazard channel
// @Description  Upgrade to WebSocket; subprotocol volunteerlink.hazards.v1.
// @Tags         conditions
// @Router       /ws/hazards [get]
func (e *ConditionsEngine) WsHazards(w http.ResponseWriter, r *http.Request) {
	conn, err := e.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// gorilla/websocket has already written the error response (403/426)
		// before returning here; calling http.Error would corrupt the response.
		return
	}
	clientID := newClientID()
	volunteerSession := r.URL.Query().Get("session")
	e.hub.Register(clientID, volunteerSession, conn)
	go func() {
		defer e.hub.Unregister(clientID)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}
