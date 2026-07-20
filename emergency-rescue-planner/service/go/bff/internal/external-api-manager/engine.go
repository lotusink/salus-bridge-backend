package external_api_manager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// ExternalAPIManager is the process-wide façade over all configured Providers.
// Construct exactly once via NewExternalAPIManager.
type ExternalAPIManager struct {
	httpClient *http.Client
	providers  map[string]ModelClient
	maxRetry   int
}

// NewExternalAPIManager builds the manager and the singleton *http.Client shared
// across all Provider implementations. At least one Provider key must be set.
func NewExternalAPIManager(cfg Config) (*ExternalAPIManager, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.MaxRetry < 0 {
		cfg.MaxRetry = 0
	}

	httpClient := &http.Client{
		Timeout: cfg.Timeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	providers := map[string]ModelClient{}
	if cfg.OpenAIKey != "" {
		providers[ProviderOpenAI] = newOpenAIClient(cfg.OpenAIKey, httpClient)
	}
	if cfg.AnthropicKey != "" {
		providers[ProviderAnthropic] = newAnthropicClient(cfg.AnthropicKey, httpClient)
	}
	if len(providers) == 0 {
		return nil, fmt.Errorf("no provider configured: set OPENAI_API_KEY and/or ANTHROPIC_API_KEY")
	}

	return &ExternalAPIManager{
		httpClient: httpClient,
		providers:  providers,
		maxRetry:   cfg.MaxRetry,
	}, nil
}

// detectProvider maps a model name to a provider key by prefix.
// Adding a new provider only requires extending this switch + the factory above.
func detectProvider(model string) (string, error) {
	switch {
	case strings.HasPrefix(model, "gpt-"),
		strings.HasPrefix(model, "whisper-"),
		strings.HasPrefix(model, "tts-"),
		strings.HasPrefix(model, "o1-"):
		return ProviderOpenAI, nil
	case strings.HasPrefix(model, "claude-"):
		return ProviderAnthropic, nil
	default:
		return "", fmt.Errorf("unknown model %q: cannot detect provider from prefix", model)
	}
}

func (m *ExternalAPIManager) routeProvider(model string) (ModelClient, error) {
	name, err := detectProvider(model)
	if err != nil {
		return nil, err
	}
	p, ok := m.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not configured", name)
	}
	return p, nil
}

// === Public methods ================================================================

// Chat performs a one-shot request with retry on transient errors.
func (m *ExternalAPIManager) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if err := req.Validate(); err != nil {
		return ChatResponse{}, err
	}
	p, err := m.routeProvider(req.Model)
	if err != nil {
		return ChatResponse{}, err
	}
	return m.withRetry(ctx, func() (ChatResponse, error) {
		return p.Chat(ctx, req)
	})
}

// Stream performs a streaming request. Streaming intentionally bypasses withRetry:
// any error fails immediately per the design decision.
func (m *ExternalAPIManager) Stream(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	p, err := m.routeProvider(req.Model)
	if err != nil {
		return nil, err
	}
	return p.Stream(ctx, req)
}

// Transcribe routes a TranscribeRequest to the Provider implementing Transcriber.
// Internal use only; not exposed via HTTP.
func (m *ExternalAPIManager) Transcribe(ctx context.Context, req TranscribeRequest) (TranscribeResponse, error) {
	if err := req.Validate(); err != nil {
		return TranscribeResponse{}, err
	}
	name, err := detectProvider(req.Model)
	if err != nil {
		return TranscribeResponse{}, err
	}
	p, ok := m.providers[name]
	if !ok {
		return TranscribeResponse{}, fmt.Errorf("provider %q not configured", name)
	}
	t, ok := p.(Transcriber)
	if !ok {
		return TranscribeResponse{}, fmt.Errorf("provider %q does not support transcription", p.Name())
	}
	return t.Transcribe(ctx, req)
}

// Synthesise routes a TTSRequest to the provider implementing Synthesiser.
func (m *ExternalAPIManager) Synthesise(ctx context.Context, req TTSRequest) (TTSResponse, error) {
	if err := req.Validate(); err != nil {
		return TTSResponse{}, err
	}
	name, err := detectProvider(req.Model)
	if err != nil {
		return TTSResponse{}, err
	}
	p, ok := m.providers[name]
	if !ok {
		return TTSResponse{}, fmt.Errorf("provider %q not configured", name)
	}
	s, ok := p.(Synthesiser)
	if !ok {
		return TTSResponse{}, fmt.Errorf("provider %q does not support speech synthesis", p.Name())
	}
	return s.Synthesise(ctx, req)
}

// withRetry retries fn up to maxRetry times when err is a *UnifiedError with
// Retryable == true. Backoff is 200ms × 2^attempt and is preempted by ctx.Done().
func (m *ExternalAPIManager) withRetry(ctx context.Context, fn func() (ChatResponse, error)) (ChatResponse, error) {
	var lastErr error
	for attempt := 0; attempt <= m.maxRetry; attempt++ {
		if ctx.Err() != nil {
			return ChatResponse{}, ctx.Err()
		}
		resp, err := fn()
		if err == nil {
			return resp, nil
		}
		var ue *UnifiedError
		if !errors.As(err, &ue) || !ue.Retryable {
			return ChatResponse{}, err
		}
		lastErr = err
		if attempt == m.maxRetry {
			break
		}
		backoff := time.Duration(200*math.Pow(2, float64(attempt))) * time.Millisecond
		select {
		case <-ctx.Done():
			return ChatResponse{}, ctx.Err()
		case <-time.After(backoff):
		}
	}
	return ChatResponse{}, lastErr
}

// === HTTP handlers =================================================================

// ChatHTTP handles a one-shot chat request.
//
// @Summary      Send a one-shot chat request to an AI provider
// @Description  Routes the chat request to OpenAI or Anthropic based on model prefix and returns the full response.
// @Tags         ai
// @Accept       json
// @Produce      json
// @Param        request body ChatRequest true "Chat Request"
// @Success      200 {object} ChatResponse
// @Failure      400 {string} string "invalid request"
// @Failure      502 {string} string "upstream error"
// @Router       /api/ai/chat [post]
func (m *ExternalAPIManager) ChatHTTP(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	defer func(Body io.ReadCloser) { _ = Body.Close() }(r.Body)

	resp, err := m.Chat(r.Context(), req)
	if err != nil {
		var ue *UnifiedError
		if errors.As(err, &ue) {
			http.Error(w, fmt.Sprintf("upstream %s error (%s)", ue.Provider, ue.Code), http.StatusBadGateway)
			return
		}
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		return
	}
}

// StreamChat streams chat completions over SSE.
//
// @Summary      Stream chat completions over SSE
// @Description  Routes the streaming chat request to OpenAI or Anthropic and forwards each delta as an SSE data frame.
// @Tags         ai
// @Accept       json
// @Produce      text/event-stream
// @Param        request body ChatRequest true "Chat Request"
// @Success      200 {string} string "SSE stream of StreamChunk frames"
// @Failure      400 {string} string "invalid request"
// @Failure      502 {string} string "upstream error"
// @Router       /api/ai/stream [post]
func (m *ExternalAPIManager) StreamChat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	defer func(Body io.ReadCloser) { _ = Body.Close() }(r.Body)

	if err := req.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}
	req.Stream = true

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	p, err := m.routeProvider(req.Model)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ch, err := p.Stream(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	for chunk := range ch {
		if chunk.Err != nil {
			payload, _ := json.Marshal(map[string]string{"message": chunk.Err.Error()})
			_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", payload)
			flusher.Flush()
			return
		}
		payload, _ := json.Marshal(chunk)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
		flusher.Flush()
		if chunk.Done {
			return
		}
	}
}
