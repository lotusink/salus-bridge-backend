package voice_search_engine

import (
	external "bff/internal/external-api-manager"
	knowledge_engine "bff/internal/knowledge-engine"
)

// VoiceSearchEngine handles POST /api/knowledge/voice-search.
// It wires together the external AI manager (transcription + intent extraction)
// and the knowledge repository (article search).
type VoiceSearchEngine struct {
	ai   *external.ExternalAPIManager
	repo knowledge_engine.Repository
}

// VoiceSearchIntent is the structured search query extracted from the transcript by Claude.
type VoiceSearchIntent struct {
	// Free-text keyword passed to the knowledge search
	Q string `json:"q"`
	// Disaster phase filter; one of "pre", "during", "post", or "" if not detected
	Phase string `json:"phase,omitempty"`
	// Disability-type filters extracted from the transcript
	DisabilityTypes []string `json:"disability_types,omitempty"`
	// Hazard filters extracted from the transcript
	Hazards []string `json:"hazards,omitempty"`
}

// VoiceSearchResponse is the body returned by POST /api/knowledge/voice-search.
// @Description Voice search result combining the raw transcript, the extracted intent, and matching articles.
type VoiceSearchResponse struct {
	// Raw text transcribed from the audio
	Transcript string `json:"transcript"`
	// Structured intent extracted by Claude; falls back to {q: transcript} when extraction fails
	Intent VoiceSearchIntent `json:"intent"`
	// Knowledge search results matching the extracted intent
	Results knowledge_engine.SearchResponse `json:"results"`
}

// transcribeRequest holds the parsed multipart fields used for the Whisper step.
type transcribeRequest struct {
	Model    string // defaults to "whisper-1"
	Language string // BCP-47 hint; empty = auto-detect
}
