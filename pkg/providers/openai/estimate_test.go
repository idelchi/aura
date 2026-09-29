package openai

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/providers"
)

func TestEstimateEndpointAvailability(t *testing.T) {
	t.Parallel()
	for _, status := range []int{200, 401, 404, 405, 501} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/responses/input_tokens" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				if status == 200 {
					_, _ = fmt.Fprint(w, `{"input_tokens":3}`)
				} else {
					_, _ = fmt.Fprint(w, `{"error":{"message":"unavailable"}}`)
				}
			}))
			defer server.Close()
			n, err := New(server.URL+"/v1", "", time.Second).Estimate(t.Context(), request.Request{Model: model.Model{Name: "route/model"}}, "Hello")
			switch status {
			case 200:
				if err != nil || n != 3 {
					t.Fatalf("estimate = %d, %v", n, err)
				}
			case 401:
				if !errors.Is(err, providers.ErrAuth) {
					t.Fatalf("lost authentication failure: %v", err)
				}
			default:
				if !errors.Is(err, providers.ErrEstimateNotSupported) {
					t.Fatalf("missing endpoint misclassified: %v", err)
				}
			}
		})
	}
}
