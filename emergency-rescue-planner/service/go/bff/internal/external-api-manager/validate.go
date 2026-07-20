package external_api_manager

import (
	"fmt"
	"regexp"
)

// toolNameRegex applies the strictest common name constraint between OpenAI and
// Anthropic so a tool definition validated here is accepted by either Provider.
var toolNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// Validate checks a ChatRequest end-to-end including embedded ToolDef entries.
func (r *ChatRequest) Validate() error {
	if r.Model == "" {
		return fmt.Errorf("model cannot be empty")
	}
	if len(r.Messages) == 0 {
		return fmt.Errorf("messages cannot be empty")
	}
	for i, msg := range r.Messages {
		switch msg.Role {
		case RoleSystem, RoleUser, RoleAssistant, RoleTool:
		default:
			return fmt.Errorf("messages[%d].role %q invalid: must be system / user / assistant / tool", i, msg.Role)
		}
		if msg.Role == RoleTool && msg.ToolCallID == "" {
			return fmt.Errorf("messages[%d].tool_call_id required when role == tool", i)
		}
	}
	if r.Temperature != nil {
		if *r.Temperature < 0 || *r.Temperature > 2 {
			return fmt.Errorf("temperature %v out of range: must be in [0, 2]", *r.Temperature)
		}
	}
	if r.MaxTokens != nil {
		if *r.MaxTokens <= 0 {
			return fmt.Errorf("max_tokens %v must be > 0", *r.MaxTokens)
		}
	}
	seenTool := make(map[string]bool, len(r.Tools))
	for i, t := range r.Tools {
		if err := t.Validate(); err != nil {
			return fmt.Errorf("tools[%d]: %w", i, err)
		}
		if seenTool[t.Name] {
			return fmt.Errorf("tools: duplicate name %q", t.Name)
		}
		seenTool[t.Name] = true
	}
	return nil
}

// Validate checks a single ToolDef. InputSchema is only minimally inspected:
// root must be an object with a "properties" key. The full JSON Schema is left
// to the upstream Provider to validate.
func (t *ToolDef) Validate() error {
	if !toolNameRegex.MatchString(t.Name) {
		return fmt.Errorf("invalid tool name %q: must match ^[a-zA-Z0-9_-]{1,64}$", t.Name)
	}
	if t.Description == "" {
		return fmt.Errorf("description cannot be empty for tool %q", t.Name)
	}
	if t.InputSchema == nil {
		return fmt.Errorf("input_schema required for tool %q", t.Name)
	}
	if typ, _ := t.InputSchema["type"].(string); typ != "object" {
		return fmt.Errorf("tool %q: input_schema root type must be \"object\"", t.Name)
	}
	if _, ok := t.InputSchema["properties"]; !ok {
		return fmt.Errorf("tool %q: input_schema must contain \"properties\"", t.Name)
	}
	return nil
}

// Validate checks a TranscribeRequest. Whisper is the only currently supported model.
func (r *TranscribeRequest) Validate() error {
	if r.Model != "whisper-1" {
		return fmt.Errorf("invalid model %q: only whisper-1 is supported", r.Model)
	}
	if r.Audio == nil {
		return fmt.Errorf("audio cannot be nil")
	}
	if r.FileName == "" {
		return fmt.Errorf("file_name cannot be empty")
	}
	return nil
}

var ttsModelSet = map[string]bool{"tts-1": true, "tts-1-hd": true}
var ttsVoiceSet = map[string]bool{
	"alloy": true, "echo": true, "fable": true,
	"onyx": true, "nova": true, "shimmer": true,
}

// Validate checks a TTSRequest.
func (r *TTSRequest) Validate() error {
	if !ttsModelSet[r.Model] {
		return fmt.Errorf("invalid model %q: must be tts-1 or tts-1-hd", r.Model)
	}
	if r.Input == "" {
		return fmt.Errorf("input cannot be empty")
	}
	if !ttsVoiceSet[r.Voice] {
		return fmt.Errorf("invalid voice %q: must be one of alloy/echo/fable/onyx/nova/shimmer", r.Voice)
	}
	if r.Speed != 0 && (r.Speed < 0.25 || r.Speed > 4.0) {
		return fmt.Errorf("speed %v out of range: must be in [0.25, 4.0]", r.Speed)
	}
	return nil
}
