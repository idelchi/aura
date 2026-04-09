package gotify

// Sandboxable returns false because the tool makes outbound HTTP requests.
func Sandboxable() bool { return false }
