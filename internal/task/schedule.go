package task

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-co-op/gocron/v2"
)

// ParseSchedule parses a schedule string with a prefix into a gocron JobDefinition.
//
// Supported prefixes:
//   - "cron: <expr>"             → CronJob (standard 5-field crontab)
//   - "every: <duration>"        → DurationJob (e.g. "every: 30m", "every: 2h")
//   - "daily: <HH:MM,...>"       → DailyJob (e.g. "daily: 09:00,17:00")
//   - "weekly: <days> <HH:MM>"   → WeeklyJob (e.g. "weekly: mon,wed,fri 09:00")
//   - "monthly: <days> <HH:MM>"  → MonthlyJob (e.g. "monthly: 1,15 09:00")
//   - "once: <RFC3339>"          → OneTimeJob (e.g. "once: 2026-03-01T09:00:00Z")
//   - "once: startup"            → OneTimeJob that fires immediately when the scheduler starts
func ParseSchedule(raw string) (gocron.JobDefinition, error) {
	// Split on ": " (colon+space) to avoid cutting inside values that contain
	// colons (e.g. RFC3339 timestamps "2026-03-01T09:00:00Z", HH:MM times in cron).
	prefix, value, ok := strings.Cut(raw, ": ")
	if !ok {
		return nil, fmt.Errorf("schedule %q: missing prefix (cron/every/daily/weekly/monthly/once)", raw)
	}

	prefix = strings.TrimSpace(strings.ToLower(prefix))
	value = strings.TrimSpace(value)

	switch prefix {
	case "cron":
		return parseCron(value)
	case "every":
		return parseEvery(value)
	case "daily":
		return parseDaily(value)
	case "weekly":
		return parseWeekly(value)
	case "monthly":
		return parseMonthly(value)
	case "once":
		return parseOnce(value)
	default:
		return nil, fmt.Errorf("unknown schedule prefix %q (valid: cron, every, daily, weekly, monthly, once)", prefix)
	}
}

func parseCron(expr string) (gocron.JobDefinition, error) {
	// Validation happens at s.NewJob() time — gocron returns an error for invalid cron expressions.
	return gocron.CronJob(expr, false), nil
}

func parseEvery(value string) (gocron.JobDefinition, error) {
	d, err := time.ParseDuration(value)
	if err != nil {
		return nil, fmt.Errorf("parsing every duration %q: %w", value, err)
	}

	return gocron.DurationJob(d), nil
}

func parseDaily(value string) (gocron.JobDefinition, error) {
	parts := strings.Split(value, ",")

	atTimes := make([]gocron.AtTime, 0, len(parts))
	for _, p := range parts {
		at, err := parseAtTime(strings.TrimSpace(p))
		if err != nil {
			return nil, fmt.Errorf("parsing daily time %q: %w", p, err)
		}

		atTimes = append(atTimes, at)
	}

	return gocron.DailyJob(1, gocron.NewAtTimes(atTimes[0], atTimes[1:]...)), nil
}

func parseWeekly(value string) (gocron.JobDefinition, error) {
	parts := strings.Fields(value)
	if len(parts) != 2 {
		return nil, fmt.Errorf("weekly schedule %q: expected '<days> <time>' (e.g. 'mon,wed,fri 09:00')", value)
	}

	days, err := parseWeekdays(parts[0])
	if err != nil {
		return nil, err
	}

	at, err := parseAtTime(parts[1])
	if err != nil {
		return nil, err
	}

	return gocron.WeeklyJob(1, gocron.NewWeekdays(days[0], days[1:]...), gocron.NewAtTimes(at)), nil
}

func parseMonthly(value string) (gocron.JobDefinition, error) {
	parts := strings.Fields(value)
	if len(parts) != 2 {
		return nil, fmt.Errorf("monthly schedule %q: expected '<days> <time>' (e.g. '1,15 09:00')", value)
	}

	dayNums, err := parseDaysOfMonth(parts[0])
	if err != nil {
		return nil, err
	}

	at, err := parseAtTime(parts[1])
	if err != nil {
		return nil, err
	}

	return gocron.MonthlyJob(1, gocron.NewDaysOfTheMonth(dayNums[0], dayNums[1:]...), gocron.NewAtTimes(at)), nil
}

func parseOnce(value string) (gocron.JobDefinition, error) {
	if strings.EqualFold(value, "startup") {
		return gocron.OneTimeJob(gocron.OneTimeJobStartDateTime(time.Now().Add(time.Second))), nil
	}

	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, fmt.Errorf("parsing once time %q (expected RFC3339 or \"startup\"): %w", value, err)
	}

	return gocron.OneTimeJob(gocron.OneTimeJobStartDateTime(t)), nil
}

func parseAtTime(s string) (gocron.AtTime, error) {
	var h, m uint

	n, err := fmt.Sscanf(s, "%d:%d", &h, &m)
	if err != nil || n != 2 {
		return gocron.NewAtTime(0, 0, 0), fmt.Errorf("time %q: expected HH:MM", s)
	}

	if h > 23 || m > 59 {
		return gocron.NewAtTime(0, 0, 0), fmt.Errorf("time %q: hour must be 0-23, minute must be 0-59", s)
	}

	return gocron.NewAtTime(h, m, 0), nil
}

var weekdayMap = map[string]time.Weekday{
	"sun": time.Sunday,
	"mon": time.Monday,
	"tue": time.Tuesday,
	"wed": time.Wednesday,
	"thu": time.Thursday,
	"fri": time.Friday,
	"sat": time.Saturday,
}

func parseWeekdays(s string) ([]time.Weekday, error) {
	parts := strings.Split(s, ",")

	days := make([]time.Weekday, 0, len(parts))
	for _, p := range parts {
		d, ok := weekdayMap[strings.TrimSpace(strings.ToLower(p))]
		if !ok {
			return nil, fmt.Errorf("unknown weekday %q (valid: sun,mon,tue,wed,thu,fri,sat)", p)
		}

		days = append(days, d)
	}

	return days, nil
}

func parseDaysOfMonth(s string) ([]int, error) {
	parts := strings.Split(s, ",")

	days := make([]int, 0, len(parts))
	for _, p := range parts {
		var d int

		if _, err := fmt.Sscanf(strings.TrimSpace(p), "%d", &d); err != nil {
			return nil, fmt.Errorf("parsing day of month %q: %w", p, err)
		}

		if d < 1 || d > 31 {
			return nil, fmt.Errorf("day of month %d: must be 1-31", d)
		}

		days = append(days, d)
	}

	return days, nil
}
