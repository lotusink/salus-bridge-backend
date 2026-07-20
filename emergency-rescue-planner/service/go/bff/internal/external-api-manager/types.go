// Package external_api_manager provides a unified client for external AI providers
// (OpenAI, Anthropic). Provider auto-detection is based on model-name prefix.
package external_api_manager

import (
	"context"
	"fmt"
	"io"
	"time"
)

// === Provider names ================================================================

const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
)

// === Role / FinishReason / ErrorCode enums =========================================

// Role is the message-role enum for a ChatMessage.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// FinishReason is the normalized completion-stop enum.
type FinishReason string

const (
	FinishStop    FinishReason = "stop"
	FinishLength  FinishReason = "length"
	FinishToolUse FinishReason = "tool_use"
	FinishError   FinishReason = "error"
)

// ErrorCode is the normalized error-code enum used in UnifiedError.
type ErrorCode string

const (
	ErrCodeRateLimit      ErrorCode = "rate_limit"
	ErrCodeInvalidAPIKey  ErrorCode = "invalid_api_key"
	ErrCodeContextTooLong ErrorCode = "context_too_long"
	ErrCodeNetworkTimeout ErrorCode = "network_timeout"
	ErrCodeInvalidRequest ErrorCode = "invalid_request"
	ErrCodeUpstream5xx    ErrorCode = "upstream_5xx"
	ErrCodeUnknown        ErrorCode = "unknown"
)

// === Configuration =================================================================

// Config is filled once at startup and passed to NewExternalAPIManager.
type Config struct {
	OpenAIKey    string
	AnthropicKey string
	Timeout      time.Duration
	MaxRetry     int
}

// === Request types =================================================================

// ChatMessage represents a single message in a conversation. Carries user input,
// assistant replies (including tool_calls), and tool-result back-fill messages.
type ChatMessage struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ChatRequest is the unified one-shot / streaming chat request.
type ChatRequest struct {
	Model       string         `json:"model"`
	Messages    []ChatMessage  `json:"messages"`
	Temperature *float64       `json:"temperature,omitempty"`
	MaxTokens   *int           `json:"max_tokens,omitempty"`
	Stream      bool           `json:"stream,omitempty"`
	Tools       []ToolDef      `json:"tools,omitempty"`
	Extra       map[string]any `json:"extra,omitempty"`
}

// ToolDef is a tool definition presented to the model.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// TranscribeRequest is the Whisper audio-transcription input. Internal use only;
// no HTTP handler exposes this.
type TranscribeRequest struct {
	Model    string
	Audio    io.Reader
	FileName string
	Language string
}

// === Response types ================================================================

// ToolCall is a tool-invocation request emitted by the model.
type ToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ChatResponse is the unified one-shot chat result.
type ChatResponse struct {
	Provider     string       `json:"provider"`
	Model        string       `json:"model"`
	Content      string       `json:"content"`
	ToolCalls    []ToolCall   `json:"tool_calls,omitempty"`
	InputTokens  int          `json:"input_tokens"`
	OutputTokens int          `json:"output_tokens"`
	FinishReason FinishReason `json:"finish_reason"`
}

// TranscribeResponse is the Whisper transcription result.
type TranscribeResponse struct {
	Text             string `json:"text"`
	DetectedLanguage string `json:"detected_language,omitempty"`
}

// === Streaming types ===============================================================

// StreamChunk is a streaming-delta envelope. The channel is closed by the
// Provider goroutine when the stream ends (success or failure).
type StreamChunk struct {
	Delta         string         `json:"delta"`
	ToolCallDelta *ToolCallDelta `json:"tool_call_delta,omitempty"`
	Done          bool           `json:"done"`
	FinishReason  FinishReason   `json:"finish_reason,omitempty"`
	Err           error          `json:"-"`
}

// ToolCallDelta is the streaming increment for a tool_use block.
type ToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	ArgsJSON string `json:"args_json,omitempty"`
}

// === Unified error =================================================================

// UnifiedError is the normalized error type all Provider implementations return.
// withRetry uses Retryable as the sole signal for whether to retry.
type UnifiedError struct {
	Provider   string
	Code       ErrorCode
	HTTPStatus int
	Raw        string
	Retryable  bool
}

// Error implements the error interface.
func (e *UnifiedError) Error() string {
	return fmt.Sprintf("[%s/%s] http=%d retryable=%v: %s",
		e.Provider, e.Code, e.HTTPStatus, e.Retryable, e.Raw)
}

// === ModelClient interface =========================================================

// ModelClient is the minimum contract every Provider must implement.
type ModelClient interface {
	Name() string
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
	Stream(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error)
}

// Transcriber is an optional capability for audio-transcription Providers.
// Only the OpenAI Whisper Provider implements it; access via type assertion.
type Transcriber interface {
	Transcribe(ctx context.Context, req TranscribeRequest) (TranscribeResponse, error)
}

// TTSRequest is the provider-layer input for OpenAI /audio/speech.
// Speed == 0 uses the provider default (1.0).
type TTSRequest struct {
	Model string  // "tts-1" | "tts-1-hd"
	Input string  // text to synthesise
	Voice string  // alloy | echo | fable | onyx | nova | shimmer
	Speed float64 // 0.25–4.0; 0 = provider default
}

// TTSResponse carries the raw audio stream from the provider.
// The caller must close Audio after use.
type TTSResponse struct {
	Audio       io.ReadCloser
	ContentType string
}

// Synthesiser is an optional capability for text-to-speech providers.
// Only the OpenAI client implements it; access via type assertion.
type Synthesiser interface {
	Synthesise(ctx context.Context, req TTSRequest) (TTSResponse, error)
}
