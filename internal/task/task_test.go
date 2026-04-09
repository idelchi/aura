package task

import (
	"strings"
	"testing"
)

func TestParseScheduleCron(t *testing.T) {
	t.Parallel()

	def, err := ParseSchedule("cron: */5 * * * *")
	if err != nil {
		t.Fatalf("ParseSchedule returned unexpected error: %v", err)
	}

	if def == nil {
		t.Errorf("ParseSchedule returned nil JobDefinition, want non-nil")
	}
}

func TestParseScheduleEvery(t *testing.T) {
	t.Parallel()

	_, err := ParseSchedule("every: 30m")
	if err != nil {
		t.Errorf("ParseSchedule returned unexpected error: %v", err)
	}
}

func TestParseScheduleDaily(t *testing.T) {
	t.Parallel()

	_, err := ParseSchedule("daily: 14:30")
	if err != nil {
		t.Errorf("ParseSchedule returned unexpected error: %v", err)
	}
}

func TestParseScheduleWeekly(t *testing.T) {
	t.Parallel()

	_, err := ParseSchedule("weekly: mon 09:00")
	if err != nil {
		t.Errorf("ParseSchedule returned unexpected error: %v", err)
	}
}

func TestParseScheduleMonthly(t *testing.T) {
	t.Parallel()

	_, err := ParseSchedule("monthly: 1,15 08:00")
	if err != nil {
		t.Errorf("ParseSchedule returned unexpected error: %v", err)
	}
}

func TestParseScheduleOnce(t *testing.T) {
	t.Parallel()

	_, err := ParseSchedule("once: 2025-12-31T00:00:00Z")
	if err != nil {
		t.Errorf("ParseSchedule returned unexpected error: %v", err)
	}
}

func TestParseScheduleInvalidPrefix(t *testing.T) {
	t.Parallel()

	_, err := ParseSchedule("bad: something")
	if err == nil {
		t.Errorf("ParseSchedule expected error for unknown prefix, got nil")
	}
}

func TestParseScheduleInvalidDuration(t *testing.T) {
	t.Parallel()

	_, err := ParseSchedule("every: notaduration")
	if err == nil {
		t.Errorf("ParseSchedule expected error for invalid duration, got nil")
	}
}

func TestParseScheduleMissingPrefix(t *testing.T) {
	t.Parallel()

	_, err := ParseSchedule("no-colon-space")
	if err == nil {
		t.Errorf("ParseSchedule expected error for missing prefix, got nil")
	}
}

func TestTaskIsManualOnly(t *testing.T) {
	t.Parallel()

	manual := Task{Name: "manual"}
	if !manual.IsManualOnly() {
		t.Errorf("IsManualOnly() = false for task with empty Schedule, want true")
	}

	scheduled := Task{Name: "scheduled", Schedule: "every: 1h"}
	if scheduled.IsManualOnly() {
		t.Errorf("IsManualOnly() = true for task with Schedule set, want false")
	}
}

func TestTaskValidateNoCommands(t *testing.T) {
	t.Parallel()

	task := Task{Name: "test"}

	err := task.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error for empty Commands, got nil")
	}

	if !strings.Contains(err.Error(), "test") {
		t.Errorf("Validate() error %q does not contain task name %q", err.Error(), "test")
	}
}

func TestTaskValidateValid(t *testing.T) {
	t.Parallel()

	task := Task{Name: "ok", Commands: []string{"echo hi"}}
	if err := task.Validate(); err != nil {
		t.Errorf("Validate() returned unexpected error: %v", err)
	}
}

func TestTaskValidateForEachNeedsSource(t *testing.T) {
	t.Parallel()

	task := Task{
		Name:     "iter",
		Commands: []string{"echo hi"},
		ForEach:  &ForEach{},
	}

	if err := task.Validate(); err == nil {
		t.Errorf("Validate() expected error for ForEach with no File or Shell, got nil")
	}
}

func TestTaskValidateForEachBothSourcesError(t *testing.T) {
	t.Parallel()

	task := Task{
		Name:     "iter",
		Commands: []string{"echo hi"},
		ForEach:  &ForEach{File: "items.txt", Shell: "echo item"},
	}

	if err := task.Validate(); err == nil {
		t.Errorf("Validate() expected error for ForEach with both File and Shell, got nil")
	}
}

func TestTaskValidateFinallyNeedsForEach(t *testing.T) {
	t.Parallel()

	task := Task{
		Name:     "iter",
		Commands: []string{"echo hi"},
		Finally:  []string{"cleanup"},
	}

	if err := task.Validate(); err == nil {
		t.Errorf("Validate() expected error for Finally without ForEach, got nil")
	}
}

func TestTasksDisplay(t *testing.T) {
	t.Parallel()

	ts := Tasks{
		"deploy": {
			Name:     "deploy",
			Schedule: "every: 1h",
			Agent:    "coder",
			Mode:     "auto",
			Commands: []string{"deploy.sh"},
		},
	}

	out := ts.Display()

	for _, want := range []string{"deploy", "enabled", "every: 1h", "NAME", "STATUS", "SCHEDULE", "----"} {
		if !strings.Contains(out, want) {
			t.Errorf("Display() = %q, missing expected substring %q", out, want)
		}
	}
}

func TestTasksGet(t *testing.T) {
	t.Parallel()

	ts := Tasks{
		"foo": {Name: "foo", Commands: []string{"echo foo"}},
	}

	got := ts.Get("foo")
	if got == nil {
		t.Errorf("Get(%q) = nil, want non-nil", "foo")
	}

	missing := ts.Get("bar")
	if missing != nil {
		t.Errorf("Get(%q) = %v, want nil", "bar", missing)
	}
}

func TestTasksNames(t *testing.T) {
	t.Parallel()

	ts := Tasks{
		"charlie": {Name: "charlie"},
		"alpha":   {Name: "alpha"},
		"bravo":   {Name: "bravo"},
	}

	names := ts.Names()

	want := []string{"alpha", "bravo", "charlie"}
	if len(names) != len(want) {
		t.Fatalf("Names() returned %d names, want %d", len(names), len(want))
	}

	for i, w := range want {
		if names[i] != w {
			t.Errorf("Names()[%d] = %q, want %q", i, names[i], w)
		}
	}
}

func TestTasksScheduled(t *testing.T) {
	t.Parallel()

	boolPtr := func(b bool) *bool { return &b }

	ts := Tasks{
		"disabled": {
			Name:     "disabled",
			Schedule: "every: 1h",
			Disabled: boolPtr(true),
			Commands: []string{"echo disabled"},
		},
		"manual": {
			Name:     "manual",
			Commands: []string{"echo manual"},
		},
		"active": {
			Name:     "active",
			Schedule: "every: 5m",
			Commands: []string{"echo active"},
		},
	}

	scheduled := ts.Scheduled()

	if len(scheduled) != 1 {
		t.Fatalf("Scheduled() returned %d tasks, want 1", len(scheduled))
	}

	if _, ok := scheduled["active"]; !ok {
		t.Errorf("Scheduled() missing %q, got keys: %v", "active", scheduled.Names())
	}
}
