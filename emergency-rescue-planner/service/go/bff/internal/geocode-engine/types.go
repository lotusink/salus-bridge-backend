package geocode_engine

// --- Nominatim wire types (unexported) ---

type nominatimSearchResult struct {
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
	DisplayName string `json:"display_name"`
}

// nominatimReverseResponse handles both success and Nominatim's HTTP-200 error body.
type nominatimReverseResponse struct {
	DisplayName string `json:"display_name"`
	Error       string `json:"error"`
}

// --- Contract request types ---

// SearchRequest holds parsed query params for GET /api/geocode/search.
type SearchRequest struct {
	Q       string
	Limit   int
	Country string
}

// ReverseRequest holds parsed query params for GET /api/geocode/reverse.
type ReverseRequest struct {
	Lat float64
	Lng float64
}

// --- Contract response wire types ---

type SearchResult struct {
	Lat   float64 `json:"lat"`
	Lng   float64 `json:"lng"`
	Label string  `json:"label"`
}

type searchResponseData struct {
	Results []SearchResult `json:"results"`
}

type SearchResponse struct {
	Data searchResponseData `json:"data"`
}

type reverseResponseData struct {
	Label string `json:"label"`
}

type ReverseResponse struct {
	Data reverseResponseData `json:"data"`
}
