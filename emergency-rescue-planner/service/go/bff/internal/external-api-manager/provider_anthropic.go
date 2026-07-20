package external_api_manager

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const anthropicVersion = "2023-06-01"

// anthropicMaxTokensDefault is used when the caller does not set MaxTokens.
// Anthropic's Messages API requires max_tokens; OpenAI does not.
const anthropicMaxTokensDefault = 4096

// anthropicClient implements ModelClient for Anthropic's Messages API. The
// *http.Client is injected by the manager.
type anthropicClient struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

func newAnthropicClient(key string, hc *http.Client) ModelClient {
	return &anthropicClient{
		apiKey:     key,
		httpClient: hc,
		baseURL:    "https://api.anthropic.com/v1",
	}
}

func (c *anthropicClient) Name() string { return ProviderAnthropic }

// === one-shot chat =================================================================

type anthropicToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// anthropicContentBlock is a polymorphic block: text / tool_use / tool_result.
// Field omitempty controls which subset is serialized for each type.
type anthropicContentBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   any            `json:"content,omitempty"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicChatBody struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Stream      bool               `json:"stream,omitempty"`
	Tools       []anthropicToolDef `json:"tools,omitempty"`
}

func toAnthropicChatBody(req ChatRequest) ([]byte, error) {
	body := anthropicChatBody{
		Model:       req.Model,
		Temperature: req.Temperature,
		Stream:      req.Stream,
	}
	if req.MaxTokens != nil {
		body.MaxTokens = *req.MaxTokens
	} else {
		body.MaxTokens = anthropicMaxTokensDefault
	}

	for _, m := range req.Messages {
		switch m.Role {
		case RoleSystem:
			if body.System != "" {
				body.System += "\n\n"
			}
			body.System += m.Content
		case RoleUser:
			body.Messages = append(body.Messages, anthropicMessage{
				Role:    "user",
				Content: []anthropicContentBlock{{Type: "text", Text: m.Content}},
			})
		case RoleAssistant:
			blocks := make([]anthropicContentBlock, 0, 1+len(m.ToolCalls))
			if m.Content != "" {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, anthropicContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: tc.Arguments,
				})
			}
			body.Messages = append(body.Messages, anthropicMessage{Role: "assistant", Content: blocks})
		case RoleTool:
			body.Messages = append(body.Messages, anthropicMessage{
				Role: "user",
				Content: []anthropicContentBlock{{
					Type:      "tool_result",
					ToolUseID: m.ToolCallID,
					Content:   m.Content,
				}},
			})
		}
	}

	for _, t := range req.Tools {
		body.Tools = append(body.Tools, anthropicToolDef{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	return json.Marshal(body)
}

type anthropicChatResponse struct {
	Model      string                  `json:"model"`
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func parseAnthropicResponse(body io.Reader) (ChatResponse, error) {
	var raw anthropicChatResponse
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return ChatResponse{}, &UnifiedError{Provider: ProviderAnthropic, Code: ErrCodeUnknown, Raw: err.Error()}
	}
	resp := ChatResponse{
		Provider:     ProviderAnthropic,
		Model:        raw.Model,
		InputTokens:  raw.Usage.InputTokens,
		OutputTokens: raw.Usage.OutputTokens,
		FinishReason: anthropicStopReason(raw.StopReason),
	}
	for _, blk := range raw.Content {
		switch blk.Type {
		case "text":
			if resp.Content != "" {
				resp.Content += "\n"
			}
			resp.Content += blk.Text
		case "tool_use":
			resp.ToolCalls = append(resp.ToolCalls, ToolCall{
				ID:        blk.ID,
				Name:      blk.Name,
				Arguments: blk.Input,
			})
		}
	}
	return resp, nil
}

func anthropicStopReason(s string) FinishReason {
	switch s {
	case "end_turn":
		return FinishStop
	case "max_tokens":
		return FinishLength
	case "tool_use":
		return FinishToolUse
	default:
		return FinishStop
	}
}

type anthropicErrorEnvelope struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func parseAnthropicError(resp *http.Response) *UnifiedError {
	body, _ := io.ReadAll(resp.Body)
	var env anthropicErrorEnvelope
	_ = json.Unmarshal(body, &env)

	ue := &UnifiedError{
		Provider:   ProviderAnthropic,
		HTTPStatus: resp.StatusCode,
		Raw:        env.Error.Message,
	}
	if ue.Raw == "" {
		ue.Raw = string(body)
	}
	switch {
	case resp.StatusCode == 401:
		ue.Code, ue.Retryable = ErrCodeInvalidAPIKey, false
	case resp.StatusCode == 429:
		ue.Code, ue.Retryable = ErrCodeRateLimit, true
	case resp.StatusCode == 400 && strings.Contains(strings.ToLower(env.Error.Message), "context"):
		ue.Code, ue.Retryable = ErrCodeContextTooLong, false
	case resp.StatusCode == 400:
		ue.Code, ue.Retryable = ErrCodeInvalidRequest, false
	case resp.StatusCode >= 500:
		ue.Code, ue.Retryable = ErrCodeUpstream5xx, true
	default:
		ue.Code, ue.Retryable = ErrCodeUnknown, false
	}
	return ue
}

func (c *anthropicClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	req.Stream = false
	body, err := toAnthropicChatBody(req)
	if err != nil {
		return ChatResponse{}, &UnifiedError{Provider: ProviderAnthropic, Code: ErrCodeUnknown, Raw: err.Error()}
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, &UnifiedError{Provider: ProviderAnthropic, Code: ErrCodeUnknown, Raw: err.Error()}
	}
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return ChatResponse{}, &UnifiedError{Provider: ProviderAnthropic, Code: ErrCodeNetworkTimeout, Retryable: true, Raw: err.Error()}
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return ChatResponse{}, parseAnthropicError(resp)
	}
	return parseAnthropicResponse(resp.Body)
}

// === streaming chat ================================================================

type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`

	Message *struct {
		Model      string `json:"model"`
		StopReason string `json:"stop_reason"`
	} `json:"message,omitempty"`

	ContentBlock *anthropicContentBlock `json:"content_block,omitempty"`

	Delta *struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta,omitempty"`
}

// anthropicBlockState tracks per-block metadata so an input_json_delta can carry
// the parent tool_use ID/Name back through the StreamChunk envelope.
type anthropicBlockState struct {
	blockType string
	id        string
	name      string
}

// parseAnthropicStreamLine decodes a single SSE line. The bool result indicates
// whether the resulting StreamChunk should be emitted (some events carry no
// payload of interest, e.g. ping / message_start / content_block_stop).
func parseAnthropicStreamLine(line string, state map[int]*anthropicBlockState) (StreamChunk, bool, error) {
	if !strings.HasPrefix(line, "data: ") {
		return StreamChunk{}, false, nil
	}
	payload := strings.TrimPrefix(line, "data: ")
	if payload == "" {
		return StreamChunk{}, false, nil
	}
	var ev anthropicStreamEvent
	if err := json.Unmarshal([]byte(payload), &ev); err != nil {
		return StreamChunk{}, false, &UnifiedError{Provider: ProviderAnthropic, Code: ErrCodeUnknown,
			Raw: fmt.Sprintf("stream parse: %v", err)}
	}
	switch ev.Type {
	case "content_block_start":
		if ev.ContentBlock != nil {
			state[ev.Index] = &anthropicBlockState{
				blockType: ev.ContentBlock.Type,
				id:        ev.ContentBlock.ID,
				name:      ev.ContentBlock.Name,
			}
			if ev.ContentBlock.Type == "tool_use" {
				return StreamChunk{ToolCallDelta: &ToolCallDelta{
					Index: ev.Index,
					ID:    ev.ContentBlock.ID,
					Name:  ev.ContentBlock.Name,
				}}, true, nil
			}
		}
		return StreamChunk{}, false, nil
	case "content_block_delta":
		if ev.Delta == nil {
			return StreamChunk{}, false, nil
		}
		switch ev.Delta.Type {
		case "text_delta":
			return StreamChunk{Delta: ev.Delta.Text}, true, nil
		case "input_json_delta":
			d := &ToolCallDelta{Index: ev.Index, ArgsJSON: ev.Delta.PartialJSON}
			if st := state[ev.Index]; st != nil {
				d.ID = st.id
				d.Name = st.name
			}
			return StreamChunk{ToolCallDelta: d}, true, nil
		}
		return StreamChunk{}, false, nil
	case "message_delta":
		if ev.Delta != nil && ev.Delta.StopReason != "" {
			return StreamChunk{Done: true, FinishReason: anthropicStopReason(ev.Delta.StopReason)}, true, nil
		}
		return StreamChunk{}, false, nil
	case "message_stop":
		return StreamChunk{Done: true, FinishReason: FinishStop}, true, nil
	}
	return StreamChunk{}, false, nil
}

func (c *anthropicClient) Stream(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error) {
	req.Stream = true
	body, err := toAnthropicChatBody(req)
	if err != nil {
		return nil, &UnifiedError{Provider: ProviderAnthropic, Code: ErrCodeUnknown, Raw: err.Error()}
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, &UnifiedError{Provider: ProviderAnthropic, Code: ErrCodeUnknown, Raw: err.Error()}
	}
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &UnifiedError{Provider: ProviderAnthropic, Code: ErrCodeNetworkTimeout, Retryable: true, Raw: err.Error()}
	}
	if resp.StatusCode != http.StatusOK {
		defer func(Body io.ReadCloser) {
			_ = Body.Close()
		}(resp.Body)
		return nil, parseAnthropicError(resp)
	}

	out := make(chan StreamChunk, 16)
	go func() {
		defer close(out)
		defer func(Body io.ReadCloser) {
			_ = Body.Close()
		}(resp.Body)

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		state := map[int]*anthropicBlockState{}

		for scanner.Scan() {
			if ctx.Err() != nil {
				out <- StreamChunk{Err: ctx.Err(), Done: true, FinishReason: FinishError}
				return
			}
			chunk, emit, err := parseAnthropicStreamLine(scanner.Text(), state)
			if err != nil {
				out <- StreamChunk{Err: err, Done: true, FinishReason: FinishError}
				return
			}
			if !emit {
				continue
			}
			out <- chunk
			if chunk.Done {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			out <- StreamChunk{Err: err, Done: true, FinishReason: FinishError}
		}
	}()
	return out, nil
}
