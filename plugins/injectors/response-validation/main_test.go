package response_validation

import (
	"context"
	"testing"

	"github.com/idelchi/aura/sdk"
)

func TestBoundedValidation(t *testing.T) {
	ctx := sdk.AfterResponseContext{}
	ctx.Iteration = 1
	ctx.Response.ValidationError = "missing answer"
	result, err := AfterResponse(context.Background(), ctx)
	if err != nil || result.Message == "" || len(result.DisableTools) != 1 {
		t.Fatalf("missing correction: %+v %v", result, err)
	}
	ctx.Iteration = 2
	result, _ = AfterResponse(context.Background(), ctx)
	if result.Stop == "" {
		t.Fatal("invalid answer did not stop")
	}
	ctx.Response.ValidationError = ""
	result, _ = AfterResponse(context.Background(), ctx)
	if result.Stop != "" || result.Message != "" {
		t.Fatal("valid answer changed")
	}
}
