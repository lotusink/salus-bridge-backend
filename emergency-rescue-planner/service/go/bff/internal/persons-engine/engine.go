package persons_engine

import (
	"encoding/json"
	"math"
	"net/http"
)

// PersonsEngine handles GET /api/vulnerable-persons.
type PersonsEngine struct {
	repo PersonRepository
}

// NewPersonsEngine creates a PersonsEngine and seeds the repository.
func NewPersonsEngine(repo *MockPersonRepository) *PersonsEngine {
	seedPersons(repo)
	return &PersonsEngine{repo: repo}
}

// GetVulnerablePersons handles GET /api/vulnerable-persons.
//
// @Summary      List vulnerable persons near a coordinate
// @Description  Returns seeded Melbourne vulnerable persons filtered by haversine distance from (lat, lng).
// @Tags         persons
// @Produce      json
// @Param        lat    query number false "Origin latitude (-90 to 90)"
// @Param        lng    query number false "Origin longitude (-180 to 180)"
// @Param        radius query number false "Search radius in km (default 10)"
// @Success      200 {object} GetVulnerablePersonsResponse
// @Failure      400 {string} string "invalid request"
// @Router       /api/vulnerable-persons [get]
func (e *PersonsEngine) GetVulnerablePersons(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	lat, latOK, err := parseOptionalFloat("lat", q.Get("lat"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	lng, lngOK, err := parseOptionalFloat("lng", q.Get("lng"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	radius, radiusOK, err := parseOptionalFloat("radius", q.Get("radius"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if latOK || lngOK {
		if err := validateLatLng(lat, lng); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if radiusOK && radius <= 0 {
		http.Error(w, "radius must be a positive number", http.StatusBadRequest)
		return
	}

	// Determine effective search radius.
	searchRadius := 10.0 // default matches frontend's default
	if !latOK && !lngOK {
		searchRadius = math.MaxFloat64 // no origin: return all persons
	} else if radiusOK {
		searchRadius = radius
	}

	persons, err := e.repo.GetPersonsNear(r.Context(), lat, lng, searchRadius)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	wires := make([]PersonWire, 0, len(persons))
	for _, p := range persons {
		wires = append(wires, personToWire(p))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(GetVulnerablePersonsResponse{
		Data: GetVulnerablePersonsData{Persons: wires},
	}); err != nil {
		return
	}
}

// personToWire converts a Person domain type to its JSON wire representation.
func personToWire(p Person) PersonWire {
	needs := p.Needs
	if needs == nil {
		needs = make([]string, 0)
	}
	return PersonWire{
		ID:             p.ID,
		Lat:            p.Lat,
		Lng:            p.Lng,
		Label:          p.Label,
		Address:        p.Address,
		Needs:          needs,
		NeedsSummary:   p.NeedsSummary,
		CtaLabel:       p.CtaLabel,
		SupportGuideID: p.SupportGuideID,
		Destination: PersonDestinationWire{
			Lat:   p.Destination.Lat,
			Lng:   p.Destination.Lng,
			Label: p.Destination.Label,
		},
	}
}
