package task

import (
	"context"
	"fmt"

	"github.com/go-co-op/gocron/v2"

	"github.com/idelchi/aura/internal/debug"
)

// RunFunc is the callback invoked for each task execution.
// It receives the task and must handle the full execution lifecycle.
type RunFunc func(ctx context.Context, t Task) error

// Scheduler wraps gocron.Scheduler and manages task job registration.
type Scheduler struct {
	s     gocron.Scheduler
	tasks Tasks
	runFn RunFunc
}

// NewScheduler creates a Scheduler with concurrency controls, registers all scheduled tasks, but does not start.
// concurrency caps the maximum number of tasks running in parallel across all jobs.
func NewScheduler(tasks Tasks, concurrency uint, runFn RunFunc) (*Scheduler, error) {
	if concurrency == 0 {
		concurrency = 1
	}

	s, err := gocron.NewScheduler(
		gocron.WithLimitConcurrentJobs(concurrency, gocron.LimitModeReschedule),
	)
	if err != nil {
		return nil, fmt.Errorf("creating scheduler: %w", err)
	}

	sched := &Scheduler{
		s:     s,
		tasks: tasks,
		runFn: runFn,
	}

	scheduled := tasks.Scheduled()
	for _, name := range scheduled.Names() {
		t := scheduled[name]
		if err := sched.register(t); err != nil {
			_ = s.Shutdown()

			return nil, fmt.Errorf("registering task %q: %w", name, err)
		}
	}

	return sched, nil
}

func (sc *Scheduler) register(t Task) error {
	jobDef, err := ParseSchedule(t.Schedule)
	if err != nil {
		return err
	}

	task := t // capture for closure

	debug.Log("[task] registered %q (schedule=%s)", task.Name, task.Schedule)

	_, err = sc.s.NewJob(
		jobDef,
		gocron.NewTask(func(ctx context.Context) {
			ctx, cancel := context.WithTimeout(ctx, task.Timeout)
			defer cancel()

			debug.Log("[task] running %q", task.Name)

			if err := sc.runFn(ctx, task); err != nil {
				debug.Log("[task] %q failed: %v", task.Name, err)
			} else {
				debug.Log("[task] %q completed", task.Name)
			}
		}),
		gocron.WithName(t.Name),
		// Prevent a task from overlapping with itself — if a trigger fires while
		// the previous run is still active, the new trigger is skipped (not queued).
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)

	return err
}

// Start begins the scheduler. Non-blocking.
func (sc *Scheduler) Start() {
	sc.s.Start()
}

// Shutdown stops the scheduler and waits for running jobs to complete.
func (sc *Scheduler) Shutdown() error {
	return sc.s.Shutdown()
}
