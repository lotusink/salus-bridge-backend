package external_api_manager

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

// openAIClient implements ModelClient + Transcriber for OpenAI's chat-completions
// and audio-transcriptions endpoints. The *http.Client is injected by the manager.
type openAIClient struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

func newOpenAIClient(key string, hc *http.Client) ModelClient {
	return &openAIClient{
		apiKey:     key,
		httpClient: hc,
		baseURL:    "https://api.openai.com/v1",
	}
}

func (c *openAIClient) Name() string { return ProviderOpenAI }

// === one-shot chat =================================================================

type openAIToolFn struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type openAIToolWrap struct {
	Type     string       `json:"type"`
	Function openAIToolFn `json:"function"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAIChatBody struct {
	Model       string           `json:"model"`
	Messages    []openAIMessage  `json:"messages"`
	Temperature *float64         `json:"temperature,omitempty"`
	MaxTokens   *int             `json:"max_tokens,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
	Tools       []openAIToolWrap `json:"tools,omitempty"`
}

func toOpenAIChatBody(req ChatRequest) ([]byte, error) {
	body := openAIChatBody{
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      req.Stream,
	}
	for _, m := range req.Messages {
		om := openAIMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			args, err := json.Marshal(tc.Arguments)
			if err != nil {
				return nil, fmt.Errorf("marshal tool_call.arguments: %w", err)
			}
			oc := openAIToolCall{ID: tc.ID, Type: "function"}
			oc.Function.Name = tc.Name
			oc.Function.Arguments = string(args)
			om.ToolCalls = append(om.ToolCalls, oc)
		}
		body.Messages = append(body.Messages, om)
	}
	for _, t := range req.Tools {
		body.Tools = append(body.Tools, openAIToolWrap{
			Type: "function",
			Function: openAIToolFn{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}
	return json.Marshal(body)
}

type openAIChatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content   string           `json:"content"`
			ToolCalls []openAIToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func parseOpenAIChatResponse(body io.Reader) (ChatResponse, error) {
	var raw openAIChatResponse
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return ChatResponse{}, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeUnknown, Raw: err.Error()}
	}
	if len(raw.Choices) == 0 {
		return ChatResponse{}, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeUnknown, Raw: "no choices in response"}
	}
	choice := raw.Choices[0]
	resp := ChatResponse{
		Provider:     ProviderOpenAI,
		Model:        raw.Model,
		Content:      choice.Message.Content,
		InputTokens:  raw.Usage.PromptTokens,
		OutputTokens: raw.Usage.CompletionTokens,
		FinishReason: openAIFinishReason(choice.FinishReason),
	}
	for _, tc := range choice.Message.ToolCalls {
		var args map[string]any
		if tc.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				return ChatResponse{}, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeUnknown,
					Raw: fmt.Sprintf("tool args parse: %v", err)}
			}
		}
		resp.ToolCalls = append(resp.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}
	return resp, nil
}

func openAIFinishReason(s string) FinishReason {
	switch s {
	case "stop":
		return FinishStop
	case "length":
		return FinishLength
	case "tool_calls":
		return FinishToolUse
	default:
		return FinishStop
	}
}

type openAIErrorEnvelope struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

func parseOpenAIError(resp *http.Response) *UnifiedError {
	body, _ := io.ReadAll(resp.Body)
	var env openAIErrorEnvelope
	_ = json.Unmarshal(body, &env)

	ue := &UnifiedError{
		Provider:   ProviderOpenAI,
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
	case resp.StatusCode == 400 && env.Error.Code == "context_length_exceeded":
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

func (c *openAIClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	req.Stream = false
	body, err := toOpenAIChatBody(req)
	if err != nil {
		return ChatResponse{}, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeUnknown, Raw: err.Error()}
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeUnknown, Raw: err.Error()}
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return ChatResponse{}, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeNetworkTimeout, Retryable: true, Raw: err.Error()}
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return ChatResponse{}, parseOpenAIError(resp)
	}
	return parseOpenAIChatResponse(resp.Body)
}

// === streaming chat ================================================================

type openAIStreamPayload struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func parseOpenAIStreamPayload(payload string) (StreamChunk, error) {
	var raw openAIStreamPayload
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return StreamChunk{}, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeUnknown,
			Raw: fmt.Sprintf("stream parse: %v", err)}
	}
	if len(raw.Choices) == 0 {
		return StreamChunk{}, nil
	}
	choice := raw.Choices[0]
	chunk := StreamChunk{Delta: choice.Delta.Content}
	if len(choice.Delta.ToolCalls) > 0 {
		tc := choice.Delta.ToolCalls[0]
		chunk.ToolCallDelta = &ToolCallDelta{
			Index:    tc.Index,
			ID:       tc.ID,
			Name:     tc.Function.Name,
			ArgsJSON: tc.Function.Arguments,
		}
	}
	if choice.FinishReason != "" {
		chunk.Done = true
		chunk.FinishReason = openAIFinishReason(choice.FinishReason)
	}
	return chunk, nil
}

func (c *openAIClient) Stream(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error) {
	req.Stream = true
	body, err := toOpenAIChatBody(req)
	if err != nil {
		return nil, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeUnknown, Raw: err.Error()}
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeUnknown, Raw: err.Error()}
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeNetworkTimeout, Retryable: true, Raw: err.Error()}
	}
	if resp.StatusCode != http.StatusOK {
		defer func(Body io.ReadCloser) {
			_ = Body.Close()
		}(resp.Body)
		return nil, parseOpenAIError(resp)
	}

	out := make(chan StreamChunk, 16)
	go func() {
		defer close(out)
		defer func(Body io.ReadCloser) {
			_ = Body.Close()
		}(resp.Body)

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		for scanner.Scan() {
			if ctx.Err() != nil {
				out <- StreamChunk{Err: ctx.Err(), Done: true, FinishReason: FinishError}
				return
			}
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := strings.TrimPrefix(line, "data: ")
			if payload == "[DONE]" {
				out <- StreamChunk{Done: true, FinishReason: FinishStop}
				return
			}
			chunk, err := parseOpenAIStreamPayload(payload)
			if err != nil {
				out <- StreamChunk{Err: err, Done: true, FinishReason: FinishError}
				return
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

// === audio transcription (Whisper) =================================================

// whisperLangToBCP47 maps the lowercase English language names returned by
// Whisper's verbose_json response to their BCP-47 codes.
// Unknown values pass through unchanged.
var whisperLangToBCP47 = map[string]string{
	"afrikaans": "af", "amharic": "am", "arabic": "ar", "assamese": "as",
	"azerbaijani": "az", "bashkir": "ba", "belarusian": "be", "bulgarian": "bg",
	"bengali": "bn", "tibetan": "bo", "breton": "br", "bosnian": "bs",
	"catalan": "ca", "czech": "cs", "welsh": "cy", "danish": "da",
	"german": "de", "greek": "el", "english": "en", "spanish": "es",
	"estonian": "et", "basque": "eu", "persian": "fa", "finnish": "fi",
	"faroese": "fo", "french": "fr", "galician": "gl", "gujarati": "gu",
	"hausa": "ha", "hawaiian": "haw", "hebrew": "he", "hindi": "hi",
	"croatian": "hr", "haitian creole": "ht", "hungarian": "hu",
	"armenian": "hy", "indonesian": "id", "icelandic": "is", "italian": "it",
	"japanese": "ja", "javanese": "jw", "georgian": "ka", "kazakh": "kk",
	"khmer": "km", "kannada": "kn", "korean": "ko", "luxembourgish": "lb",
	"lingala": "ln", "lao": "lo", "latin": "la", "lithuanian": "lt",
	"latvian": "lv", "malagasy": "mg", "maori": "mi", "macedonian": "mk",
	"malayalam": "ml", "mongolian": "mn", "marathi": "mr", "malay": "ms",
	"maltese": "mt", "myanmar": "my", "nepali": "ne", "dutch": "nl",
	"nynorsk": "nn", "norwegian": "no", "occitan": "oc", "punjabi": "pa",
	"polish": "pl", "pashto": "ps", "portuguese": "pt", "romanian": "ro",
	"russian": "ru", "sanskrit": "sa", "sindhi": "sd", "sinhala": "si",
	"slovak": "sk", "slovenian": "sl", "shona": "sn", "somali": "so",
	"albanian": "sq", "serbian": "sr", "sundanese": "su", "swedish": "sv",
	"swahili": "sw", "tamil": "ta", "tajik": "tg", "thai": "th",
	"turkmen": "tk", "tagalog": "tl", "turkish": "tr", "tatar": "tt",
	"ukrainian": "uk", "urdu": "ur", "uzbek": "uz", "vietnamese": "vi",
	"yoruba": "yo", "chinese": "zh",
}

func whisperLangCode(word string) string {
	if code, ok := whisperLangToBCP47[word]; ok {
		return code
	}
	return word
}

func (c *openAIClient) Transcribe(ctx context.Context, req TranscribeRequest) (TranscribeResponse, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("model", req.Model); err != nil {
		return TranscribeResponse{}, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeUnknown, Raw: err.Error()}
	}
	if err := mw.WriteField("response_format", "verbose_json"); err != nil {
		return TranscribeResponse{}, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeUnknown, Raw: err.Error()}
	}
	if req.Language != "" {
		if err := mw.WriteField("language", req.Language); err != nil {
			return TranscribeResponse{}, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeUnknown, Raw: err.Error()}
		}
	}
	fw, err := mw.CreateFormFile("file", req.FileName)
	if err != nil {
		return TranscribeResponse{}, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeUnknown, Raw: err.Error()}
	}
	if _, err := io.Copy(fw, req.Audio); err != nil {
		return TranscribeResponse{}, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeUnknown, Raw: err.Error()}
	}
	if err := mw.Close(); err != nil {
		return TranscribeResponse{}, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeUnknown, Raw: err.Error()}
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/audio/transcriptions", &buf)
	if err != nil {
		return TranscribeResponse{}, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeUnknown, Raw: err.Error()}
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return TranscribeResponse{}, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeNetworkTimeout, Retryable: true, Raw: err.Error()}
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return TranscribeResponse{}, parseOpenAIError(resp)
	}
	type openAITranscribeVerbose struct {
		Text     string `json:"text"`
		Language string `json:"language"`
	}
	var raw openAITranscribeVerbose
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return TranscribeResponse{}, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeUnknown, Raw: err.Error()}
	}
	return TranscribeResponse{Text: raw.Text, DetectedLanguage: whisperLangCode(raw.Language)}, nil
}

// === text-to-speech (TTS) ===========================================================

type openAITTSBody struct {
	Model string  `json:"model"`
	Input string  `json:"input"`
	Voice string  `json:"voice"`
	Speed float64 `json:"speed,omitempty"`
}

func (c *openAIClient) Synthesise(ctx context.Context, req TTSRequest) (TTSResponse, error) {
	body := openAITTSBody{
		Model: req.Model,
		Input: req.Input,
		Voice: req.Voice,
		Speed: req.Speed,
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return TTSResponse{}, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeUnknown, Raw: err.Error()}
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/audio/speech", bytes.NewReader(encoded))
	if err != nil {
		return TTSResponse{}, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeUnknown, Raw: err.Error()}
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return TTSResponse{}, &UnifiedError{Provider: ProviderOpenAI, Code: ErrCodeNetworkTimeout, Retryable: true, Raw: err.Error()}
	}
	if resp.StatusCode != http.StatusOK {
		defer func(Body io.ReadCloser) {
			_ = Body.Close()
		}(resp.Body)
		return TTSResponse{}, parseOpenAIError(resp)
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "audio/mpeg"
	}
	return TTSResponse{Audio: resp.Body, ContentType: ct}, nil
}
