package translation_engine

import external "bff/internal/external-api-manager"

// TranslationEngine handles the three STT / translate / TTS HTTP routes.
type TranslationEngine struct {
	ai *external.ExternalAPIManager
}

// NewTranslationEngine creates a TranslationEngine backed by the given manager.
func NewTranslationEngine(ai *external.ExternalAPIManager) *TranslationEngine {
	return &TranslationEngine{ai: ai}
}

// TTSCache is a no-op placeholder for future TTS response caching.
type TTSCache struct{}

// TranscribeRequest holds fields parsed from a multipart/form-data body.
// Not decoded from JSON; built manually in the handler from r.FormValue / r.FormFile.
type TranscribeRequest struct {
	// Whisper model to use; handler defaults to "whisper-1"
	Model string
	// Optional BCP-47 language hint for the input audio
	Language string
}

// TranscribeResponse is the JSON body returned by POST /api/knowledge/transcribe.
// @Description Transcription result including the BCP-47 detected language code.
type TranscribeResponse struct {
	// Full transcribed text from the audio file
	Text string `json:"text" example:"Residents in the flood zone must evacuate before 6 pm."`
	// BCP-47 language code detected by Whisper (e.g. en, zh, fr)
	DetectedLanguage string `json:"detected_language,omitempty" example:"en"`
}

// TranslateRequest is the JSON body for POST /api/knowledge/translate.
// @Description Translation input: source text, BCP-47 target language, and optional model override.
type TranslateRequest struct {
	// Source text to translate
	Text string `json:"text" example:"Residents in the flood zone must evacuate before 6 pm."`
	// BCP-47 target language code
	TargetLanguage string `json:"target_language" example:"zh-CN"`
	// Claude model to use; defaults to claude-haiku-4-5
	Model string `json:"model,omitempty" example:"claude-haiku-4-5"`
}

// TranslateResponse is the JSON body returned by POST /api/knowledge/translate.
// DetectedSourceLanguage is reserved for a future iteration and always omitted for now.
// @Description Translation result.
type TranslateResponse struct {
	// Translated text in the requested target language
	TranslatedText string `json:"translated_text" example:"洪水区居民必须在下午6点前撤离。"`
	// BCP-47 source language code detected during translation (reserved; not populated)
	DetectedSourceLanguage string `json:"detected_source_language,omitempty" example:"en"`
}

// TTSRequest is the JSON body for POST /api/knowledge/tts.
// @Description TTS input: text to speak, voice selection, and optional model / speed overrides.
type TTSRequest struct {
	// Text to synthesise into speech
	Text string `json:"text" example:"Residents in the flood zone must evacuate before 6 pm."`
	// Voice ID to use; defaults to alloy
	Voice string `json:"voice,omitempty" example:"alloy" enums:"alloy,echo,fable,onyx,nova,shimmer"`
	// TTS model; defaults to tts-1
	Model string `json:"model,omitempty" example:"tts-1" enums:"tts-1,tts-1-hd"`
	// Speech speed multiplier (0.25–4.0); 0 uses the provider default (1.0)
	Speed float64 `json:"speed,omitempty" example:"1.0"`
}

// LanguageOption represents a single supported translation target language.
// @Description BCP-47 language code and its English display name.
type LanguageOption struct {
	// BCP-47 language code passed as target_language to POST /api/knowledge/translate
	Code string `json:"code" example:"zh-CN"`
	// English display name shown in the language-selection UI
	Name string `json:"name" example:"Simplified Chinese"`
}
