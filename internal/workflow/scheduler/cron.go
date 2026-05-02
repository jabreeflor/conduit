// Package scheduler provides a cron-based trigger for Conduit workflows.
//
// The cron parser supports the standard 5-field syntax
//
//	minute hour day-of-month month day-of-week
//
// where each field accepts:
//
//   - any value
//     N           a literal number
//     A-B         a range (inclusive)
//     A,B,C       a list
//     */N         a step (every N starting at the field minimum)
//     A-B/N       a stepped range
//
// Day-of-week accepts 0-6 with both 0 and 7 meaning Sunday. The parser also
// recognizes the descriptor shortcuts @hourly, @daily, @midnight (alias of
// @daily), @weekly, @monthly, and @yearly/@annually.
package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// fieldRange describes the legal value range for a cron field.
type fieldRange struct {
	min int
	max int
}

var (
	rangeMinute     = fieldRange{0, 59}
	rangeHour       = fieldRange{0, 23}
	rangeDayOfMonth = fieldRange{1, 31}
	rangeMonth      = fieldRange{1, 12}
	// Day-of-week accepts 0-7 with 7 normalised to 0 (Sunday) after parsing.
	rangeDayOfWeek = fieldRange{0, 7}
)

// cronSchedule is a parsed cron expression.
type cronSchedule struct {
	minute     map[int]struct{}
	hour       map[int]struct{}
	dayOfMonth map[int]struct{}
	month      map[int]struct{}
	dayOfWeek  map[int]struct{}
	// domStar and dowStar mark whether the original field was "*". A standard
	// cron rule says: when both DOM and DOW are restricted, the schedule fires
	// when *either* matches (logical OR). When one is "*", only the other is
	// considered.
	domStar bool
	dowStar bool
}

// parseExpression parses a 5-field cron expression or a supported descriptor.
func parseExpression(expr string) (*cronSchedule, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("cron: empty expression")
	}
	if strings.HasPrefix(expr, "@") {
		return parseDescriptor(expr)
	}

	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron: expected 5 fields, got %d in %q", len(fields), expr)
	}

	minute, _, err := parseField(fields[0], rangeMinute)
	if err != nil {
		return nil, fmt.Errorf("cron: minute: %w", err)
	}
	hour, _, err := parseField(fields[1], rangeHour)
	if err != nil {
		return nil, fmt.Errorf("cron: hour: %w", err)
	}
	dom, domStar, err := parseField(fields[2], rangeDayOfMonth)
	if err != nil {
		return nil, fmt.Errorf("cron: day-of-month: %w", err)
	}
	month, _, err := parseField(fields[3], rangeMonth)
	if err != nil {
		return nil, fmt.Errorf("cron: month: %w", err)
	}
	dow, dowStar, err := parseField(fields[4], rangeDayOfWeek)
	if err != nil {
		return nil, fmt.Errorf("cron: day-of-week: %w", err)
	}
	// 7 is an accepted alias for Sunday in many cron variants. Normalise it.
	if _, ok := dow[7]; ok {
		dow[0] = struct{}{}
		delete(dow, 7)
	}

	return &cronSchedule{
		minute:     minute,
		hour:       hour,
		dayOfMonth: dom,
		month:      month,
		dayOfWeek:  dow,
		domStar:    domStar,
		dowStar:    dowStar,
	}, nil
}

func parseDescriptor(expr string) (*cronSchedule, error) {
	switch strings.ToLower(expr) {
	case "@hourly":
		return parseExpression("0 * * * *")
	case "@daily", "@midnight":
		return parseExpression("0 0 * * *")
	case "@weekly":
		return parseExpression("0 0 * * 0")
	case "@monthly":
		return parseExpression("0 0 1 * *")
	case "@yearly", "@annually":
		return parseExpression("0 0 1 1 *")
	default:
		return nil, fmt.Errorf("cron: unsupported descriptor %q", expr)
	}
}

// parseField parses a single field spec, returning the set of allowed values
// and whether the spec was the bare wildcard "*". The 7-field-alternate-cron
// extension is *not* supported.
func parseField(spec string, r fieldRange) (map[int]struct{}, bool, error) {
	if spec == "" {
		return nil, false, fmt.Errorf("empty field")
	}
	star := spec == "*" || strings.HasPrefix(spec, "*/")
	values := map[int]struct{}{}
	for _, part := range strings.Split(spec, ",") {
		if part == "" {
			return nil, false, fmt.Errorf("empty list entry in %q", spec)
		}
		if err := expandPart(part, r, values); err != nil {
			return nil, false, err
		}
	}
	if len(values) == 0 {
		return nil, false, fmt.Errorf("no values produced for %q", spec)
	}
	return values, star, nil
}

func expandPart(part string, r fieldRange, out map[int]struct{}) error {
	step := 1
	rangeSpec := part
	if i := strings.Index(part, "/"); i >= 0 {
		rangeSpec = part[:i]
		stepStr := part[i+1:]
		if stepStr == "" {
			return fmt.Errorf("missing step value in %q", part)
		}
		var err error
		step, err = strconv.Atoi(stepStr)
		if err != nil || step <= 0 {
			return fmt.Errorf("invalid step %q in %q", stepStr, part)
		}
	}

	var lo, hi int
	switch {
	case rangeSpec == "*":
		lo, hi = r.min, r.max
	case strings.Contains(rangeSpec, "-"):
		bounds := strings.SplitN(rangeSpec, "-", 2)
		var err error
		lo, err = strconv.Atoi(bounds[0])
		if err != nil {
			return fmt.Errorf("invalid range start %q", bounds[0])
		}
		hi, err = strconv.Atoi(bounds[1])
		if err != nil {
			return fmt.Errorf("invalid range end %q", bounds[1])
		}
	default:
		v, err := strconv.Atoi(rangeSpec)
		if err != nil {
			return fmt.Errorf("invalid value %q", rangeSpec)
		}
		// Bare integer with a step (e.g. "5/15") expands to [5, max].
		if step != 1 {
			lo, hi = v, r.max
		} else {
			lo, hi = v, v
		}
	}

	if lo < r.min || hi > r.max {
		return fmt.Errorf("value out of range [%d-%d] in %q", r.min, r.max, part)
	}
	if lo > hi {
		return fmt.Errorf("inverted range %d-%d in %q", lo, hi, part)
	}
	for v := lo; v <= hi; v += step {
		out[v] = struct{}{}
	}
	return nil
}

// matches reports whether the given time matches all cron fields.
func (c *cronSchedule) matches(t time.Time) bool {
	if _, ok := c.minute[t.Minute()]; !ok {
		return false
	}
	if _, ok := c.hour[t.Hour()]; !ok {
		return false
	}
	if _, ok := c.month[int(t.Month())]; !ok {
		return false
	}
	_, domOK := c.dayOfMonth[t.Day()]
	// Go's Weekday: Sunday=0 ... Saturday=6 — already aligned with cron.
	_, dowOK := c.dayOfWeek[int(t.Weekday())]
	switch {
	case c.domStar && c.dowStar:
		return true
	case c.domStar:
		return dowOK
	case c.dowStar:
		return domOK
	default:
		// Both restricted: vixie-cron OR semantics.
		return domOK || dowOK
	}
}

// next returns the smallest time strictly after `after` that matches the
// schedule. The search is bounded to four years so impossible specs (e.g.
// Feb 30) terminate.
func (c *cronSchedule) next(after time.Time) (time.Time, error) {
	t := after.Add(time.Minute - time.Duration(after.Nanosecond())*time.Nanosecond)
	t = t.Add(-time.Duration(t.Second()) * time.Second)
	if !t.After(after) {
		t = t.Add(time.Minute)
	}
	limit := after.AddDate(4, 0, 0)
	for t.Before(limit) || t.Equal(limit) {
		if c.matches(t) {
			return t, nil
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("cron: no matching time within 4 years of %s", after.Format(time.RFC3339))
}

// Next returns the next firing time strictly after `after` for the supplied
// expression. The expression is parsed on every call; callers that fire often
// should cache via parseExpression directly.
func Next(expr string, after time.Time) (time.Time, error) {
	sched, err := parseExpression(expr)
	if err != nil {
		return time.Time{}, err
	}
	return sched.next(after)
}
