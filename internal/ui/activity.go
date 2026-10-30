package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/idelchi/aura/pkg/llm/part"
	"github.com/idelchi/aura/pkg/llm/tool/call"
)

// ActivityQuiet is how long without observed progress before the activity reports the
// gap and stops naming a specific model phase: the backend does not stream tool
// arguments, so a silent model may be generating them, or anything else.
const ActivityQuiet = 3 * time.Second

// Awaiting identifies user input the assistant is blocked on.
type Awaiting int

const (
	// AwaitingNothing means no dialog is open.
	AwaitingNothing Awaiting = iota
	// AwaitingConfirmation means a tool call waits for the user's approval.
	AwaitingConfirmation
	// AwaitingAnswer means the assistant asked the user a question.
	AwaitingAnswer
)

// ActivityState classifies what the assistant is doing, for presentation.
type ActivityState int

const (
	// ActivityReady means nothing has run yet.
	ActivityReady ActivityState = iota
	// ActivityWorking means a turn or a background job is running.
	ActivityWorking
	// ActivityAwaitingUser means the assistant is blocked on the user.
	ActivityAwaitingUser
	// ActivityDone means the last turn completed.
	ActivityDone
	// ActivityFailed means the last turn ended with an error.
	ActivityFailed
	// ActivityCancelled means the last turn was cancelled.
	ActivityCancelled
)

// modelPhase is what the current model call last showed.
type modelPhase int

const (
	modelWaiting  modelPhase = iota // request pending or between iterations, nothing streamed yet
	modelThinking                   // thinking deltas arrived
	modelWriting                    // content deltas arrived
	modelFinished                   // message finalized while the turn goes on
)

// trackedTool is a tool call registered in the current turn and not yet finished.
type trackedTool struct {
	id      string
	name    string
	running bool
}

// Activity follows the UI event stream to tell what the assistant is doing for the
// whole turn: between tools, between messages and while nothing streams.
// It only reports what events show — elapsed time is not proof of provider progress.
// Interactive UIs feed it every event (Observe) and their own dialog state (AwaitUser).
type Activity struct {
	turn       bool          // a turn is in progress: UserMessagesProcessed .. AssistantDone
	started    time.Time     // when the turn started
	progress   time.Time     // when progress was last observed
	model      modelPhase    // state of the current model call
	tools      []trackedTool // unfinished tool calls of the turn
	note       string        // latest backend phase text, superseded by later message events
	awaiting   Awaiting      // dialog the UI has open
	cancelling bool          // the user asked to cancel the turn
	last       ActivityState // outcome of the last turn (Done, Failed, Cancelled) or Ready
	took       time.Duration // duration of the last turn
}

// Observe updates the activity from an event received at now.
func (a *Activity) Observe(event Event, now time.Time) {
	switch e := event.(type) {
	case UserMessagesProcessed:
		a.begin(now)
	case MessageStarted:
		a.progress = now
		a.model = modelWaiting
		a.note = ""
	case MessagePartAdded:
		a.observePart(e.Part, now)
	case MessagePartUpdated:
		a.observePart(e.Part, now)
	case MessageFinalized:
		a.progress = now
		a.model = modelFinished
		a.note = ""
		a.tools = nil
	case ToolOutputDelta:
		a.progress = now
	case SpinnerMessage:
		a.progress = now
		a.note = strings.TrimRight(strings.TrimSpace(e.Text), ".…")
	case SlashCommandHandled:
		// A finished command no longer owns the phase text it may have set.
		if !e.Forwarded {
			a.note = ""
		}
	case AssistantDone:
		a.end(e, now)
	}
}

// AwaitUser records which dialog the UI has open. Answering restarts the progress clock,
// so the user's own time is not reported as a silent backend.
func (a *Activity) AwaitUser(awaiting Awaiting, now time.Time) {
	if awaiting == a.awaiting {
		return
	}

	if awaiting == AwaitingNothing {
		a.progress = now
	}

	a.awaiting = awaiting
}

// RequestCancel records that the user asked to cancel the running turn.
func (a *Activity) RequestCancel() {
	if a.turn {
		a.cancelling = true
	}
}

// InTurn reports whether a turn is in progress, including while it waits for the user.
func (a Activity) InTurn() bool {
	return a.turn
}

// State classifies the activity.
func (a Activity) State() ActivityState {
	switch {
	case a.awaiting != AwaitingNothing:
		return ActivityAwaitingUser
	case a.turn || a.note != "":
		return ActivityWorking
	default:
		return a.last
	}
}

// Quiet returns how long no progress has been observed.
func (a Activity) Quiet(now time.Time) time.Duration {
	return now.Sub(a.progress)
}

// Display returns a one-line description, e.g. "Running Bash · 1m5s · last update 40s ago".
func (a Activity) Display(now time.Time) string {
	switch a.State() {
	case ActivityAwaitingUser:
		label := "Waiting for your answer"
		if a.awaiting == AwaitingConfirmation {
			label = "Waiting for your confirmation"
		}

		if a.turn {
			return label + " · " + roundDuration(now.Sub(a.started))
		}

		return label
	case ActivityWorking:
		parts := []string{a.phase(now)}

		if a.turn {
			parts = append(parts, roundDuration(now.Sub(a.started)))
		}

		if quiet := a.Quiet(now); quiet >= ActivityQuiet {
			parts = append(parts, fmt.Sprintf("last update %s ago", roundDuration(quiet)))
		}

		return strings.Join(parts, " · ")
	case ActivityDone:
		return "Done in " + roundDuration(a.took)
	case ActivityFailed:
		return "Failed after " + roundDuration(a.took)
	case ActivityCancelled:
		return "Cancelled after " + roundDuration(a.took)
	default:
		return "Ready"
	}
}

// phase names what the turn is doing, from the most specific signal available.
func (a Activity) phase(now time.Time) string {
	if a.cancelling {
		return "Cancelling"
	}

	if a.note != "" {
		return a.note
	}

	var running []string

	for _, t := range a.tools {
		if t.running {
			running = append(running, t.name)
		}
	}

	switch {
	case len(running) == 1:
		return "Running " + running[0]
	case len(running) > 1:
		return fmt.Sprintf("Running %d tools", len(running))
	case len(a.tools) > 0:
		return "Preparing tools"
	}

	fresh := a.Quiet(now) < ActivityQuiet

	switch a.model {
	case modelThinking:
		if fresh {
			return "Thinking"
		}

		return "Generating response"
	case modelWriting:
		if fresh {
			return "Writing response"
		}

		return "Generating response"
	case modelFinished:
		return "Working"
	default:
		return "Waiting for model"
	}
}

func (a *Activity) begin(now time.Time) {
	*a = Activity{
		turn:     true,
		started:  now,
		progress: now,
		awaiting: a.awaiting,
	}
}

func (a *Activity) end(e AssistantDone, now time.Time) {
	last := ActivityDone

	switch {
	case e.Cancelled:
		last = ActivityCancelled
	case e.Error != nil:
		last = ActivityFailed
	}

	took := now.Sub(a.started)
	if !a.turn {
		took = 0
	}

	*a = Activity{
		progress: now,
		awaiting: a.awaiting,
		last:     last,
		took:     took,
	}
}

func (a *Activity) observePart(p part.Part, now time.Time) {
	a.progress = now
	a.note = ""

	switch p.Type {
	case part.Thinking:
		a.model = modelThinking
	case part.Content:
		a.model = modelWriting
	case part.Tool:
		if p.Call == nil {
			return
		}

		i := slices.IndexFunc(a.tools, func(t trackedTool) bool { return t.id == p.Call.ID })

		switch p.Call.State {
		case call.Complete, call.Error:
			if i >= 0 {
				a.tools = slices.Delete(a.tools, i, i+1)
			}

			if len(a.tools) == 0 {
				// The iteration's tools are done; the next thing is the model again (or the turn's end).
				a.model = modelWaiting
			}
		default:
			running := p.Call.State == call.Running

			if i >= 0 {
				a.tools[i].running = running
			} else {
				a.tools = append(a.tools, trackedTool{id: p.Call.ID, name: p.Call.Name, running: running})
			}
		}
	}
}

// roundDuration formats d to whole seconds, e.g. "1m5s".
func roundDuration(d time.Duration) string {
	return max(d, 0).Truncate(time.Second).String()
}
