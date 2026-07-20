package translation_engine

import "fmt"

// Validate checks a TranscribeRequest. Only whisper-1 is supported.
func (r *TranscribeRequest) Validate() error {
	if r.Model != "whisper-1" {
		return fmt.Errorf("invalid model %q: only whisper-1 is supported", r.Model)
	}
	return nil
}

// Validate checks a TranslateRequest.
func (r *TranslateRequest) Validate() error {
	if r.Text == "" {
		return fmt.Errorf("text cannot be empty")
	}
	if r.TargetLanguage == "" {
		return fmt.Errorf("target_language cannot be empty")
	}
	return nil
}

var ttsVoiceSet = map[string]bool{
	"alloy": true, "echo": true, "fable": true,
	"onyx": true, "nova": true, "shimmer": true,
}

// Validate checks a TTSRequest. Called after the handler has applied defaults,
// so Model and Voice are guaranteed non-empty.
func (r *TTSRequest) Validate() error {
	if r.Text == "" {
		return fmt.Errorf("text cannot be empty")
	}
	if r.Model != "tts-1" && r.Model != "tts-1-hd" {
		return fmt.Errorf("invalid model %q: must be tts-1 or tts-1-hd", r.Model)
	}
	if !ttsVoiceSet[r.Voice] {
		return fmt.Errorf("invalid voice %q: must be one of alloy/echo/fable/onyx/nova/shimmer", r.Voice)
	}
	if r.Speed != 0 && (r.Speed < 0.25 || r.Speed > 4.0) {
		return fmt.Errorf("speed %v out of range: must be in [0.25, 4.0]", r.Speed)
	}
	return nil
}
