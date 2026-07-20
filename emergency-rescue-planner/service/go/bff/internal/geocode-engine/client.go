package geocode_engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const (
	nominatimBaseURL   = "https://nominatim.openstreetmap.org"
	nominatimTimeout   = 30 * time.Second
	nominatimUserAgent = "VolunteerLink/1.0 (volunteerlink-bff)"
)

// rateLimiter enforces at most one Nominatim request per second.
// Concurrent callers queue behind the mutex and sleep to fill the gap.
type rateLimiter struct {
	mu          sync.Mutex
	lastRequest time.Time
	interval    time.Duration
}

func (rl *rateLimiter) Wait() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	elapsed := time.Since(rl.lastRequest)
	if elapsed < rl.interval {
		time.Sleep(rl.interval - elapsed)
	}
	rl.lastRequest = time.Now()
}

// NominatimClient calls the Nominatim public geocoding API.
// Rate-limited to 1 req/sec; all requests include the required User-Agent header.
type NominatimClient struct {
	httpClient *http.Client
	limiter    *rateLimiter
}

// NewNominatimClient creates a NominatimClient with a 30 s timeout and 1 req/sec limiter.
func NewNominatimClient() *NominatimClient {
	return &NominatimClient{
		httpClient: &http.Client{Timeout: nominatimTimeout},
		limiter:    &rateLimiter{interval: time.Second},
	}
}

// Search calls Nominatim /search and returns contract SearchResult slice.
// Returns an empty (non-nil) slice when Nominatim finds no results.
func (c *NominatimClient) Search(ctx context.Context, req SearchRequest) ([]SearchResult, error) {
	c.limiter.Wait()

	params := url.Values{}
	params.Set("q", req.Q)
	params.Set("format", "json")
	params.Set("limit", strconv.Itoa(req.Limit))
	params.Set("countrycodes", req.Country)

	httpReq, err := http.NewRequestWithContext(ctx, "GET",
		nominatimBaseURL+"/search?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("build nominatim request: %w", err)
	}
	httpReq.Header.Set("User-Agent", nominatimUserAgent)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("nominatim search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("nominatim search %d: %s", resp.StatusCode, string(b))
	}

	var raw []nominatimSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode nominatim search: %w", err)
	}

	results := make([]SearchResult, 0, len(raw))
	for _, r := range raw {
		lat, err := strconv.ParseFloat(r.Lat, 64)
		if err != nil {
			continue // skip malformed entry; do not fail the whole response
		}
		lng, err := strconv.ParseFloat(r.Lon, 64)
		if err != nil {
			continue
		}
		results = append(results, SearchResult{Lat: lat, Lng: lng, Label: r.DisplayName})
	}
	return results, nil
}

// Reverse calls Nominatim /reverse and returns the display_name label.
// Returns error when Nominatim cannot geocode the coordinates.
func (c *NominatimClient) Reverse(ctx context.Context, req ReverseRequest) (string, error) {
	c.limiter.Wait()

	params := url.Values{}
	params.Set("lat", strconv.FormatFloat(req.Lat, 'f', -1, 64))
	params.Set("lon", strconv.FormatFloat(req.Lng, 'f', -1, 64))
	params.Set("format", "json")

	httpReq, err := http.NewRequestWithContext(ctx, "GET",
		nominatimBaseURL+"/reverse?"+params.Encode(), nil)
	if err != nil {
		return "", fmt.Errorf("build nominatim reverse request: %w", err)
	}
	httpReq.Header.Set("User-Agent", nominatimUserAgent)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("nominatim reverse: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("nominatim reverse %d: %s", resp.StatusCode, string(b))
	}

	var result nominatimReverseResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode nominatim reverse: %w", err)
	}

	// Nominatim returns HTTP 200 with {"error": "..."} when no result found.
	if result.Error != "" {
		return "", fmt.Errorf("nominatim reverse: %s", result.Error)
	}

	return result.DisplayName, nil
}
