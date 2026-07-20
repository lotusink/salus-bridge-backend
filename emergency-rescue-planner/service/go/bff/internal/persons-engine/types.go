package persons_engine

// Person is the domain representation of a vulnerable person.
// No JSON or DB tags — wire conversion is done in engine.go.
type Person struct {
	ID             string
	Label          string
	Address        string   // street-style address; "" when unknown
	Needs          []string // e.g. ["mobility"], ["autism"]
	NeedsSummary   string
	CtaLabel       string
	SupportGuideID *string // nil → JSON null; pointer required by contract
	Lat            float64
	Lng            float64
	Destination    PersonDestination
}

// PersonDestination is the evacuation destination for a person.
type PersonDestination struct {
	Lat   float64
	Lng   float64
	Label string
}

// --- Wire types (JSON tags; used only in HTTP responses) ---

// PersonWire is the JSON representation of Person for HTTP responses.
type PersonWire struct {
	ID             string                `json:"id"`
	Lat            float64               `json:"lat"`
	Lng            float64               `json:"lng"`
	Label          string                `json:"label"`
	Address        string                `json:"address"`
	Needs          []string              `json:"needs"`
	NeedsSummary   string                `json:"needs_summary"`
	CtaLabel       string                `json:"cta_label"`
	SupportGuideID *string               `json:"support_guide_id"`
	Destination    PersonDestinationWire `json:"destination"`
}

// PersonDestinationWire is the JSON representation of PersonDestination.
type PersonDestinationWire struct {
	Lat   float64 `json:"lat"`
	Lng   float64 `json:"lng"`
	Label string  `json:"label"`
}

// GetVulnerablePersonsData is the data envelope for GET /api/vulnerable-persons.
type GetVulnerablePersonsData struct {
	Persons []PersonWire `json:"persons"`
}

// GetVulnerablePersonsResponse is the full response for GET /api/vulnerable-persons.
type GetVulnerablePersonsResponse struct {
	Data GetVulnerablePersonsData `json:"data"`
}
