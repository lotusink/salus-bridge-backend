package voice_search_engine

import "fmt"

// validate checks that model is a supported Whisper model identifier.
// Called after the handler fills the default ("whisper-1") for an absent model field.
func validate(model string) error {
	if model != "whisper-1" {
		return fmt.Errorf("invalid model %q: only whisper-1 is supported", model)
	}
	return nil
}
