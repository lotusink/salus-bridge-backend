// Package health implements the /health_check endpoint following
// draft-inadarei-api-health-check-06 (application/health+json). The handler
// returns a JSON document covering service identity, uptime, dependency
// state, and the list of routes registered on the running mux.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Pinger is the contract the health engine requires from a database
// connector: a context-aware liveness probe.
type Pinger interface {
	PingContext(ctx context.Context) error
}

// EnvSnapshot captures the presence (not the value) of configurable third-
// party integrations. The snapshot is taken once at construction time —
// runtime mutation of environment variables is not reflected.
type EnvSnapshot struct {
	OpenAIKey    bool
	AnthropicKey bool
	ORSKey       bool
}

// Config bundles the dependencies HealthEngine needs at construction.
type Config struct {
	ServiceID   string
	Description string
	Version     string
	ReleaseID   string
	StartedAt   time.Time
	DBPinger    Pinger
	EnvSnapshot EnvSnapshot
	Routes      []Route
	PingTimeout time.Duration // defaults to 2s when zero
}

// HealthEngine renders the application/health+json response.
type HealthEngine struct {
	cfg Config
}

// NewHealthEngine constructs a HealthEngine. Routes are sorted in place by
// (path, method) so the response is stable across calls.
func NewHealthEngine(cfg Config) *HealthEngine {
	if cfg.PingTimeout == 0 {
		cfg.PingTimeout = 2 * time.Second
	}
	sortRoutes(cfg.Routes)
	return &HealthEngine{cfg: cfg}
}

// GetHealth returns service health in the application/health+json format.
//
// @Summary      Health check
// @Description  Returns service status, dependency state, uptime, and registered routes.
// @Tags         system
// @Produce      application/health+json
// @Success      200  {object}  health.Response
// @Failure      503  {object}  health.Response
// @Router       /health_check [get]
func (e *HealthEngine) GetHealth(w http.ResponseWriter, r *http.Request) {
	resp := e.build(r.Context())

	httpCode := http.StatusOK
	if resp.Status == StatusFail {
		httpCode = http.StatusServiceUnavailable
	}

	body, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "failed to encode health response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", MediaType)
	w.WriteHeader(httpCode)
	_, _ = w.Write(body)
}

func (e *HealthEngine) build(ctx context.Context) Response {
	checks := map[string][]CheckItem{}

	// Database ping — hard dependency. A failure forces overall status=fail.
	dbItem, dbFailOutput := e.dbCheck(ctx)
	checks["database:ping"] = []CheckItem{dbItem}

	// Optional integrations — env-presence only, never live-pinged.
	openai := configuredCheck(e.cfg.EnvSnapshot.OpenAIKey, "set", "unset")
	checks["openai_api:configured"] = []CheckItem{openai}
	anthropic := configuredCheck(e.cfg.EnvSnapshot.AnthropicKey, "set", "unset")
	checks["anthropic_api:configured"] = []CheckItem{anthropic}
	ors := configuredCheck(e.cfg.EnvSnapshot.ORSKey, "set", "unset")
	checks["ors_api:configured"] = []CheckItem{ors}

	status := aggregate(dbItem.Status, []string{openai.Status, anthropic.Status, ors.Status})

	resp := Response{
		Status:        status,
		Version:       e.cfg.Version,
		ReleaseID:     e.cfg.ReleaseID,
		ServiceID:     e.cfg.ServiceID,
		Description:   e.cfg.Description,
		Checks:        checks,
		UptimeSeconds: e.uptime(),
		GoVersion:     runtime.Version(),
		Routes:        append([]Route(nil), e.cfg.Routes...),
	}
	if status == StatusFail {
		resp.Output = dbFailOutput
	}
	return resp
}

func (e *HealthEngine) uptime() int64 {
	if e.cfg.StartedAt.IsZero() {
		return 0
	}
	return int64(time.Since(e.cfg.StartedAt).Seconds())
}

// dbCheck pings the DB; the second return value is the raw error message
// to surface in the top-level Output field on failure.
func (e *HealthEngine) dbCheck(ctx context.Context) (CheckItem, string) {
	if e.cfg.DBPinger == nil {
		return CheckItem{
			Status:        StatusWarn,
			ComponentType: "datastore",
			ObservedValue: "no pinger configured",
		}, ""
	}
	pingCtx, cancel := context.WithTimeout(ctx, e.cfg.PingTimeout)
	defer cancel()
	if err := e.cfg.DBPinger.PingContext(pingCtx); err != nil {
		return CheckItem{
			Status:        StatusFail,
			ComponentType: "datastore",
			ObservedValue: "ping failed",
			Output:        err.Error(),
		}, err.Error()
	}
	return CheckItem{
		Status:        StatusPass,
		ComponentType: "datastore",
		ObservedValue: "ok",
	}, ""
}

// configuredCheck reports pass when an optional integration's env var is
// set, warn when unset.
func configuredCheck(present bool, presentValue, missingValue string) CheckItem {
	if present {
		return CheckItem{
			Status:        StatusPass,
			ComponentType: "component",
			ObservedValue: presentValue,
		}
	}
	return CheckItem{
		Status:        StatusWarn,
		ComponentType: "component",
		ObservedValue: missingValue,
	}
}

// aggregate produces the overall status: DB failure short-circuits to fail;
// any optional warn promotes the result to warn; otherwise pass.
func aggregate(db string, optional []string) string {
	if db == StatusFail {
		return StatusFail
	}
	for _, s := range optional {
		if s == StatusWarn {
			return StatusWarn
		}
	}
	return StatusPass
}

// ParseRoutePattern splits a Go 1.22+ mux pattern of the form "METHOD /path"
// into a Route. When the method prefix is omitted, Method is "ANY".
func ParseRoutePattern(pattern string) Route {
	pattern = strings.TrimSpace(pattern)
	if idx := strings.IndexByte(pattern, ' '); idx > 0 {
		head := pattern[:idx]
		tail := strings.TrimSpace(pattern[idx+1:])
		// Heuristic: an HTTP method is uppercase letters only.
		if isHTTPMethod(head) {
			return Route{Method: head, Path: tail}
		}
	}
	return Route{Method: "ANY", Path: pattern}
}

func isHTTPMethod(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func sortRoutes(routes []Route) {
	sort.SliceStable(routes, func(i, j int) bool {
		if routes[i].Path != routes[j].Path {
			return routes[i].Path < routes[j].Path
		}
		return routes[i].Method < routes[j].Method
	})
}
