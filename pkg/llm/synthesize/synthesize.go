// Package synthesize defines common types for text-to-speech operations.
package synthesize

// Request represents a text-to-speech request.
type Request struct {
	// Model is the TTS model name.
	Model string
	// Input is the text to convert to speech.
	Input string
	// Voice is the voice identifier (e.g. "alloy", "af_heart").
	Voice string
	// Format is the output audio format (e.g. "mp3", "wav", "opus").
	Format string
	// Speed controls playback speed (0.25-4.0, default 1.0).
	Speed float64
}

// Response represents a text-to-speech response.
type Response struct {
	// Audio is the raw audio bytes.
	Audio []byte
}
