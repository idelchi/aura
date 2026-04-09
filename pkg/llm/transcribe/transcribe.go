// Package transcribe defines common types for speech-to-text operations.
package transcribe

// Request represents a transcription request.
type Request struct {
	// Model is the model name to use for transcription.
	Model string
	// FilePath is the path to the audio file to transcribe.
	FilePath string
	// Language is an optional ISO-639-1 language hint (e.g. "en").
	Language string
}

// Response represents a transcription response.
type Response struct {
	// Text is the transcribed text content.
	Text string
}
