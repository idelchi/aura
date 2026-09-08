package call

import (
	"errors"
	"strings"
	"testing"
)

// TestResultDistinguishesExecutionFromAdmission keeps output rejection separate
// from execution failure, without granting small payloads special treatment.
func TestResultDistinguishesExecutionFromAdmission(t *testing.T) {
	if got := (Result{Output: "sent"}).String(); got != "sent" {
		t.Fatal(got)
	}
	got := (Result{Output: "private payload", Omission: "budget exhausted"}).String()
	if !strings.Contains(got, "executed successfully") || !strings.Contains(got, "budget exhausted") || strings.Contains(got, "private payload") {
		t.Fatal(got)
	}
	got = (Result{Err: errors.New("invalid arguments"), Omission: "budget exhausted"}).String()
	if got != "Error: invalid arguments" {
		t.Fatal(got)
	}
}
