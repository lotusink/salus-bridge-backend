package health

// Status values defined by draft-inadarei-api-health-check-06.
const (
	StatusPass = "pass"
	StatusWarn = "warn"
	StatusFail = "fail"
)

// MediaType is the Content-Type recommended by the spec.
const MediaType = "application/health+json"

// Response is the top-level health document. Field names follow the IETF
// draft (camelCase), with `uptime_seconds`, `go_version`, and `routes`
// added as non-spec extensions for internal observability.
type Response struct {
	Status        string                 `json:"status"`
	Version       string                 `json:"version,omitempty"`
	ReleaseID     string                 `json:"releaseId,omitempty"`
	ServiceID     string                 `json:"serviceId,omitempty"`
	Description   string                 `json:"description,omitempty"`
	Notes         []string               `json:"notes,omitempty"`
	Output        string                 `json:"output,omitempty"`
	Checks        map[string][]CheckItem `json:"checks,omitempty"`
	UptimeSeconds int64                  `json:"uptime_seconds"`
	GoVersion     string                 `json:"go_version,omitempty"`
	Routes        []Route                `json:"routes,omitempty"`
}

// CheckItem is one row of a checks entry per the spec.
type CheckItem struct {
	Status        string `json:"status"`
	ComponentType string `json:"componentType,omitempty"`
	ObservedValue string `json:"observedValue,omitempty"`
	Output        string `json:"output,omitempty"`
}

// Route is a single registered HTTP route. Method is "ANY" when the
// registered pattern omitted the method prefix.
type Route struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}
