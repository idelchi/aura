package llamacpp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/openai/openai-go/v3"

	"github.com/idelchi/aura/pkg/providers"
	"github.com/idelchi/aura/pkg/providers/adapter"
	"github.com/idelchi/aura/pkg/truncate"
)

// Bounds of the diagnostics carried in errors.
const (
	maxErrorBody   = 64 << 10 // bytes read from an error response body
	maxDiagnostic  = 300      // bytes of body excerpt kept in the error message
	exceedsContext = "exceed_context_size_error"
)

// Send performs a request against a native (non-OpenAI-compatible) endpoint and classifies
// its failures like inference failures: a transport failure is a network error
// (cancellation stays a cancellation) and a non-2xx response becomes the provider error of
// its status class, wrapping a StatusError with the status and a bounded diagnostic of the
// body. On success the caller owns and must close the response body.
func (c *Client) Send(req *http.Request) (*http.Response, error) {
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, transportError(req.Context(), err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()

		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))

		return nil, c.statusError(req, resp, body)
	}

	return resp, nil
}

// statusError classifies a failed response by its status code alone.
func (c *Client) statusError(req *http.Request, resp *http.Response, body []byte) error {
	message := fmt.Sprintf("%s %s returned %s", req.Method, req.URL.Path, resp.Status)
	if detail := describe(resp, body); detail != "" {
		message += ": " + detail
	}

	return providers.ClassifyHTTPError(resp.StatusCode, c.name, message, providers.ParseRetryAfter(resp))
}

// transportError types a failed round trip. Cancellation by the caller is kept as is.
func transportError(ctx context.Context, err error) error {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return err
	}

	return providers.WrapNetworkError(err)
}

// describe returns the server's error message (llama.cpp and llama-swap answer
// {"error":{"type","message"}} or {"error":"…"}), otherwise a bounded, whitespace-collapsed
// excerpt of the body with its content type. Classification never depends on it.
func describe(resp *http.Response, body []byte) string {
	var structured struct {
		Error json.RawMessage `json:"error"`
	}

	if json.Unmarshal(body, &structured) == nil && len(structured.Error) > 0 {
		var detailed struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}

		if json.Unmarshal(structured.Error, &detailed) == nil && detailed.Message != "" {
			if detailed.Type != "" {
				return truncate.Truncate(detailed.Type+": "+detailed.Message, maxDiagnostic)
			}

			return truncate.Truncate(detailed.Message, maxDiagnostic)
		}

		var text string
		if json.Unmarshal(structured.Error, &text) == nil && text != "" {
			return truncate.Truncate(text, maxDiagnostic)
		}
	}

	return excerpt(resp, body)
}

// excerpt summarises an unstructured body: its content type and leading text.
func excerpt(resp *http.Response, body []byte) string {
	text := strings.Join(strings.Fields(string(body)), " ")
	if text == "" {
		return ""
	}

	if kind := resp.Header.Get("Content-Type"); kind != "" {
		return truncate.Truncate(kind+" body: "+text, maxDiagnostic)
	}

	return truncate.Truncate(text, maxDiagnostic)
}

// decodeJSON decodes a successful response, reporting a body that is not the expected
// JSON with its content type and a bounded excerpt.
func decodeJSON(req *http.Request, resp *http.Response, v any) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return transportError(req.Context(), err)
	}

	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("%s %s returned %s that is not the expected JSON (%w): %s",
			req.Method, req.URL.Path, resp.Status, err, excerpt(resp, body))
	}

	return nil
}

// inferenceError classifies a failure of the OpenAI-compatible inference API. HTTP errors
// map by status (and llama.cpp's structured context-size type); others go through the
// shared adapter, which types network failures.
func (c *Client) inferenceError(err error) error {
	apiErr, ok := errors.AsType[*openai.Error](err)
	if !ok {
		return adapter.MapError(err)
	}

	detail := apiErr.Message
	if detail == "" {
		detail = strings.Join(strings.Fields(apiErr.RawJSON()), " ")
	}

	if apiErr.Type != "" && detail != "" {
		detail = apiErr.Type + ": " + detail
	}

	message := fmt.Sprintf("%d %s", apiErr.StatusCode, http.StatusText(apiErr.StatusCode))
	if apiErr.Request != nil {
		message = fmt.Sprintf("%s %s returned %s", apiErr.Request.Method, apiErr.Request.URL.Path, message)
	}

	if detail != "" {
		message += ": " + truncate.Truncate(detail, maxDiagnostic)
	}

	status := &providers.StatusError{Provider: c.name, Status: apiErr.StatusCode, Message: message, Err: apiErr}

	if apiErr.Type == exceedsContext {
		return fmt.Errorf("%w: %w", providers.ErrContextExhausted, status)
	}

	return status.Classified(providers.ParseRetryAfter(apiErr.Response))
}
