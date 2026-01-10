package timerange

import (
	"fmt"
	"strings"
	"time"
)

type Flags struct {
	From      string
	To        string
	Today     bool
	Tomorrow  bool
	Week      bool
	Days      int
	WeekStart string
}

type Defaults struct {
	FromOffset   time.Duration
	ToOffset     time.Duration
	ToFromOffset time.Duration
}

type Range struct {
	From     time.Time
	To       time.Time
	Location *time.Location
}

func Resolve(now time.Time, loc *time.Location, flags Flags, defaults Defaults) (*Range, error) {
	if loc == nil {
		loc = time.UTC
	}
	now = now.In(loc)
	var from, to time.Time

	weekStart, err := resolveWeekStart(flags.WeekStart)
	if err != nil {
		return nil, err
	}

	switch {
	case flags.Today:
		from = startOfDay(now)
		to = endOfDay(now)
	case flags.Tomorrow:
		tomorrow := now.AddDate(0, 0, 1)
		from = startOfDay(tomorrow)
		to = endOfDay(tomorrow)
	case flags.Week:
		from = startOfWeek(now, weekStart)
		to = endOfWeek(now, weekStart)
	case flags.Days > 0:
		from = startOfDay(now)
		to = endOfDay(now.AddDate(0, 0, flags.Days-1))
	default:
		if flags.From != "" {
			from, err = parseTimeExpr(flags.From, now, loc)
			if err != nil {
				return nil, fmt.Errorf("invalid --from: %w", err)
			}
		} else {
			from = now.Add(defaults.FromOffset)
		}

		switch {
		case flags.To != "":
			to, err = parseTimeExpr(flags.To, now, loc)
			if err != nil {
				return nil, fmt.Errorf("invalid --to: %w", err)
			}
		case flags.From != "" && defaults.ToFromOffset != 0:
			to = from.Add(defaults.ToFromOffset)
		default:
			to = now.Add(defaults.ToOffset)
		}
	}

	return &Range{From: from, To: to, Location: loc}, nil
}

func parseTimeExpr(expr string, now time.Time, loc *time.Location) (time.Time, error) {
	expr = strings.TrimSpace(expr)
	if t, err := time.Parse(time.RFC3339, expr); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02T15:04:05-0700", expr); err == nil {
		return t, nil
	}

	exprLower := strings.ToLower(expr)
	switch exprLower {
	case "now":
		return now, nil
	case "today":
		return startOfDay(now), nil
	case "tomorrow":
		return startOfDay(now.AddDate(0, 0, 1)), nil
	case "yesterday":
		return startOfDay(now.AddDate(0, 0, -1)), nil
	}

	if t, ok := parseWeekday(exprLower, now); ok {
		return t, nil
	}

	if t, err := time.ParseInLocation("2006-01-02", expr, loc); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", expr, loc); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04", expr, loc); err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("cannot parse %q as time", expr)
}

func parseWeekday(expr string, now time.Time) (time.Time, bool) {
	expr = strings.TrimSpace(expr)
	next := false
	if strings.HasPrefix(expr, "next ") {
		next = true
		expr = strings.TrimPrefix(expr, "next ")
	}

	weekdays := map[string]time.Weekday{
		"sunday":    time.Sunday,
		"sun":       time.Sunday,
		"monday":    time.Monday,
		"mon":       time.Monday,
		"tuesday":   time.Tuesday,
		"tue":       time.Tuesday,
		"wednesday": time.Wednesday,
		"wed":       time.Wednesday,
		"thursday":  time.Thursday,
		"thu":       time.Thursday,
		"friday":    time.Friday,
		"fri":       time.Friday,
		"saturday":  time.Saturday,
		"sat":       time.Saturday,
	}

	targetDay, ok := weekdays[expr]
	if !ok {
		return time.Time{}, false
	}

	currentDay := now.Weekday()
	daysUntil := int(targetDay) - int(currentDay)
	if daysUntil < 0 || (daysUntil == 0 && next) {
		daysUntil += 7
	}
	return startOfDay(now.AddDate(0, 0, daysUntil)), true
}

func resolveWeekStart(value string) (time.Weekday, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return time.Monday, nil
	}
	switch value {
	case "sun", "sunday":
		return time.Sunday, nil
	case "mon", "monday":
		return time.Monday, nil
	case "tue", "tuesday":
		return time.Tuesday, nil
	case "wed", "wednesday":
		return time.Wednesday, nil
	case "thu", "thursday":
		return time.Thursday, nil
	case "fri", "friday":
		return time.Friday, nil
	case "sat", "saturday":
		return time.Saturday, nil
	default:
		return time.Monday, fmt.Errorf("invalid week start %q", value)
	}
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func endOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), t.Location())
}

func startOfWeek(t time.Time, weekStart time.Weekday) time.Time {
	days := (int(t.Weekday()) - int(weekStart) + 7) % 7
	return startOfDay(t.AddDate(0, 0, -days))
}

func endOfWeek(t time.Time, weekStart time.Weekday) time.Time {
	start := startOfWeek(t, weekStart)
	return endOfDay(start.AddDate(0, 0, 6))
}
