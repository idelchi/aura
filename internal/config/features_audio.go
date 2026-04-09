package config

// STT holds configuration for speech-to-text (transcription).
type STT struct {
	// Agent is the name of the agent to use for transcription calls.
	Agent string `yaml:"agent"`
	// Language is the default ISO-639-1 language hint (e.g. "en"). Empty = auto-detect.
	Language string `yaml:"language"`
}

// ApplyDefaults is a no-op — STT has no default values.
func (s *STT) ApplyDefaults() error { return nil }

// TTS holds configuration for text-to-speech (synthesis).
type TTS struct {
	// Agent is the name of the agent to use for synthesis calls.
	Agent string `yaml:"agent"`
	// Voice is the default voice identifier (e.g. "alloy", "af_heart").
	Voice string `yaml:"voice"`
	// Format is the default output audio format (e.g. "mp3", "wav").
	Format string `yaml:"format"`
	// Speed is the default playback speed (0.25-4.0). Default: 1.0.
	Speed float64 `yaml:"speed"`
}

// ApplyDefaults sets sane defaults for zero-valued fields.
func (t *TTS) ApplyDefaults() error {
	if t.Voice == "" {
		t.Voice = "alloy"
	}

	if t.Format == "" {
		t.Format = "mp3"
	}

	if t.Speed == 0 {
		t.Speed = 1.0
	}

	return nil
}
