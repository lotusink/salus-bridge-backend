package field_report_engine

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"
)

// ConfirmedReportInfo carries the data needed for cluster detection after a confirm.
type ConfirmedReportInfo struct {
	ReportID  string
	TypeID    string
	Lat       float64
	Lng       float64
	CreatedAt time.Time
}

// ClusterNotifier receives notification after a field report is confirmed.
type ClusterNotifier interface {
	OnReportConfirmed(ctx context.Context, info ConfirmedReportInfo)
}

type FieldReportEngine struct {
	store           FieldReportStore
	clusterNotifier ClusterNotifier // nil until SetClusterNotifier called
}

func NewFieldReportEngine() *FieldReportEngine {
	s := NewMemStore()
	seedReports(s)
	seedConfirmations(s)
	return &FieldReportEngine{store: s}
}

func (e *FieldReportEngine) SetClusterNotifier(n ClusterNotifier) {
	e.clusterNotifier = n
}

// Store returns the underlying FieldReportStore for cluster-engine access.
func (e *FieldReportEngine) Store() FieldReportStore { return e.store }

func newReportID() string {
	return fmt.Sprintf("rpt-%d-%d", time.Now().UnixNano(), rand.Int63n(10000))
}

// GetReports handles GET /api/field-reports.
//
// @Summary      List nearby field reports
// @Tags         field-reports
// @Produce      json
// @Param        lat      query number true  "Origin latitude"
// @Param        lng      query number true  "Origin longitude"
// @Param        dest_lat query number true  "Destination latitude"
// @Param        dest_lng query number true  "Destination longitude"
// @Success      200 {object} GetReportsResponse
// @Failure      400 {string} string "invalid request"
// @Router       /api/field-reports [get]
func (e *FieldReportEngine) GetReports(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	// Validate all four coordinate params (v1: not used for filtering — see tech debt in 2-plan.md)
	for _, p := range []struct{ name, val string }{
		{"lat", q.Get("lat")}, {"lng", q.Get("lng")},
		{"dest_lat", q.Get("dest_lat")}, {"dest_lng", q.Get("dest_lng")},
	} {
		if _, err := parseFloat(p.name, p.val); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}
	}
	lat, _ := parseFloat("lat", q.Get("lat"))
	lng, _ := parseFloat("lng", q.Get("lng"))
	destLat, _ := parseFloat("dest_lat", q.Get("dest_lat"))
	destLng, _ := parseFloat("dest_lng", q.Get("dest_lng"))
	if err := validateLatLng("lat", "lng", lat, lng); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}
	if err := validateLatLng("dest_lat", "dest_lng", destLat, destLng); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	records := e.store.GetAll()
	summaries := make([]ReportSummary, 0, len(records))
	for _, rec := range records {
		summaries = append(summaries, ReportSummary{
			ID:             rec.ID,
			TypeID:         rec.TypeID,
			Location:       rec.Location,
			Body:           rec.Body,
			ConfirmedCount: rec.ConfirmedCount,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(GetReportsResponse{
		Data: GetReportsResponseData{Reports: summaries},
	}); err != nil {
		return
	}
}

// SubmitReport handles POST /api/field-reports.
//
// @Summary      Submit a new field report
// @Tags         field-reports
// @Accept       json
// @Produce      json
// @Param        X-Volunteer-Session header string true "Session ID for dedupe"
// @Param        request body SubmitReportRequest true "Report"
// @Success      201 {object} SubmitReportResponse
// @Failure      400 {string} string "invalid request"
// @Router       /api/field-reports [post]
func (e *FieldReportEngine) SubmitReport(w http.ResponseWriter, r *http.Request) {
	session := r.Header.Get("X-Volunteer-Session")
	if session == "" {
		http.Error(w, "X-Volunteer-Session header is required", http.StatusBadRequest)
		return
	}

	var req SubmitReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err := ValidateSubmitRequest(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// severity=High → confidence=high immediately (contract rule — ARCHITECTURE.md)
	confidence := ConfidenceLow
	if req.Severity == "High" {
		confidence = ConfidenceHigh
	}

	rec := fieldReportRecord{
		ID:             newReportID(),
		TypeID:         req.TypeID,
		Body:           req.Body,
		Location:       req.Location,
		Label:          fmt.Sprintf("%s at %s", typeIDToLabel(req.TypeID), req.Location),
		Lat:            req.Lat,
		Lng:            req.Lng,
		Severity:       req.Severity,
		Confidence:     confidence,
		ConfirmedCount: 0,
		CreatorSession: session,
		CreatedAt:      time.Now(),
	}
	stored := e.store.Create(rec)

	detail := ReportDetail{
		ID:             stored.ID,
		Label:          stored.Label,
		CreatedAt:      stored.CreatedAt.UTC().Format(time.RFC3339),
		Location:       stored.Location,
		TypeID:         stored.TypeID,
		Body:           stored.Body,
		ConfirmedCount: stored.ConfirmedCount,
		IsMine:         true,
		Confidence:     stored.Confidence,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(SubmitReportResponse{
		Data: SubmitReportResponseData{
			Report:     detail,
			Confidence: stored.Confidence,
		},
	}); err != nil {
		return
	}
}

// ConfirmReport handles POST /api/field-reports/{id}/confirm.
//
// @Summary      Confirm (peer-verify) a field report
// @Tags         field-reports
// @Accept       json
// @Produce      json
// @Param        id                  path   string true  "Report ID"
// @Param        X-Volunteer-Session header string true  "Session ID for dedupe"
// @Success      200 {object} ConfirmReportResponse
// @Failure      400 {string} string "invalid request"
// @Failure      404 {string} string "report not found"
// @Failure      409 {string} string "already voted"
// @Router       /api/field-reports/{id}/confirm [post]
func (e *FieldReportEngine) ConfirmReport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "report id is required", http.StatusBadRequest)
		return
	}

	session := r.Header.Get("X-Volunteer-Session")
	if session == "" {
		http.Error(w, "X-Volunteer-Session header is required", http.StatusBadRequest)
		return
	}

	rec, err := e.store.Confirm(id, session)
	if err == ErrReportNotFound {
		http.Error(w, "report not found", http.StatusNotFound)
		return
	}
	if err == ErrAlreadyVoted {
		http.Error(w, "already voted", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Cluster check: run synchronously so promotion happens before response is sent.
	if e.clusterNotifier != nil {
		e.clusterNotifier.OnReportConfirmed(r.Context(), ConfirmedReportInfo{
			ReportID:  rec.ID,
			TypeID:    rec.TypeID,
			Lat:       rec.Lat,
			Lng:       rec.Lng,
			CreatedAt: rec.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(ConfirmReportResponse{
		Data: ConfirmReportResponseData{
			ConfirmedCount: rec.ConfirmedCount,
			Confidence:     rec.Confidence,
			// affected_route is always false in v1 — deliberate stub.
			// Becomes meaningful in 5-realtime-hazard-channel when active-route
			// registration (POST /api/routes/active) is introduced (ADR §4 / D8).
			AffectedRoute: false,
		},
	}); err != nil {
		return
	}
}
