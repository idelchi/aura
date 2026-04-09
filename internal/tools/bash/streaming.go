package bash

import (
	"bytes"
	"sync"
	"time"
)

const streamThrottle = 200 * time.Millisecond

// StreamingWriter wraps a LimitedBuffer and calls a callback with each complete
// line of output, throttled to avoid flooding the UI event channel.
// Thread-safe: shell pipelines may write from multiple goroutines.
type StreamingWriter struct {
	buf      *LimitedBuffer
	callback func(line string)
	lineBuf  []byte
	lastLine string
	mu       sync.Mutex
	lastEmit time.Time
}

// NewStreamingWriter creates a writer that tees to buf and streams lines via callback.
func NewStreamingWriter(buf *LimitedBuffer, callback func(line string)) *StreamingWriter {
	return &StreamingWriter{
		buf:      buf,
		callback: callback,
	}
}

// Write implements io.Writer. It always writes to the underlying LimitedBuffer,
// then splits on newlines and calls the callback for complete lines (throttled).
func (sw *StreamingWriter) Write(p []byte) (int, error) {
	n, err := sw.buf.Write(p)

	sw.mu.Lock()
	defer sw.mu.Unlock()

	if !sw.buf.Capped() {
		sw.lineBuf = append(sw.lineBuf, p[:n]...)
	}

	for {
		idx := bytes.IndexByte(sw.lineBuf, '\n')
		if idx < 0 {
			break
		}

		line := string(sw.lineBuf[:idx])

		sw.lineBuf = sw.lineBuf[idx+1:]

		now := time.Now()
		if now.Sub(sw.lastEmit) >= streamThrottle {
			sw.callback(line)

			sw.lastEmit = now
			sw.lastLine = ""
		} else {
			sw.lastLine = line
		}
	}

	// Emit the most recent throttled line so the UI catches up.
	if sw.lastLine != "" {
		sw.callback(sw.lastLine)

		sw.lastLine = ""
		sw.lastEmit = time.Now()
	}

	return n, err
}

// Flush emits any remaining partial line (content without a trailing newline).
// Call after the command finishes to ensure the last line is not lost.
func (sw *StreamingWriter) Flush() {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(sw.lineBuf) > 0 {
		sw.callback(string(sw.lineBuf))

		sw.lineBuf = nil
	}
}
