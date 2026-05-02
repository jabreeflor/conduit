package scheduler

import (
	"testing"
	"time"
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse time %q: %v", s, err)
	}
	return v.UTC()
}

func TestNext_Basic(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		expr string
		from string
		want string
	}{
		{
			name: "every minute fires next minute",
			expr: "* * * * *",
			from: "2025-05-02T10:30:15Z",
			want: "2025-05-02T10:31:00Z",
		},
		{
			name: "step minutes */5 from :03 lands at :05",
			expr: "*/5 * * * *",
			from: "2025-05-02T10:03:00Z",
			want: "2025-05-02T10:05:00Z",
		},
		{
			name: "list at quarter hours from :17 lands at :30",
			expr: "0,15,30,45 * * * *",
			from: "2025-05-02T10:17:00Z",
			want: "2025-05-02T10:30:00Z",
		},
		{
			name: "range 1-5 weekday Tue->Wed",
			expr: "0 9 * * 1-5",
			from: "2025-05-06T09:00:30Z", // Tue 09:00:30
			want: "2025-05-07T09:00:00Z", // Wed 09:00
		},
		{
			name: "@hourly fires next top-of-hour",
			expr: "@hourly",
			from: "2025-05-02T10:30:00Z",
			want: "2025-05-02T11:00:00Z",
		},
		{
			name: "@daily at midnight",
			expr: "@daily",
			from: "2025-05-02T10:30:00Z",
			want: "2025-05-03T00:00:00Z",
		},
		{
			name: "@weekly fires next Sunday at 00:00",
			expr: "@weekly",
			from: "2025-05-02T10:30:00Z", // Friday
			want: "2025-05-04T00:00:00Z", // Sunday
		},
		{
			name: "@monthly fires first of next month",
			expr: "@monthly",
			from: "2025-05-02T10:30:00Z",
			want: "2025-06-01T00:00:00Z",
		},
		{
			name: "leap day skip on non-leap year goes to 2028",
			expr: "0 0 29 2 *",
			from: "2025-01-01T00:00:00Z",
			want: "2028-02-29T00:00:00Z",
		},
		{
			name: "stepped range 1-10/3 hours yields 1,4,7,10",
			expr: "0 1-10/3 * * *",
			from: "2025-05-02T05:00:00Z",
			want: "2025-05-02T07:00:00Z",
		},
		{
			name: "DOW alias 7 = Sunday",
			expr: "0 0 * * 7",
			from: "2025-05-02T10:00:00Z", // Friday
			want: "2025-05-04T00:00:00Z", // Sunday
		},
		{
			name: "DOM and DOW both restricted: OR semantics",
			expr: "0 0 13 * 5", // 13th of the month OR Friday
			from: "2025-05-02T10:00:00Z",
			want: "2025-05-09T00:00:00Z", // next Friday
		},
		{
			name: "year rollover from Dec 31 23:59",
			expr: "0 0 1 1 *",
			from: "2025-12-31T23:59:00Z",
			want: "2026-01-01T00:00:00Z",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Next(tc.expr, mustTime(t, tc.from))
			if err != nil {
				t.Fatalf("Next(%q) error: %v", tc.expr, err)
			}
			want := mustTime(t, tc.want)
			if !got.Equal(want) {
				t.Fatalf("Next(%q) from %s = %s, want %s", tc.expr, tc.from, got.Format(time.RFC3339), want.Format(time.RFC3339))
			}
		})
	}
}

func TestNext_StrictlyAfter(t *testing.T) {
	t.Parallel()
	// When `after` already matches the spec, Next must advance.
	at := mustTime(t, "2025-05-02T10:00:00Z")
	got, err := Next("0 10 * * *", at)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	want := mustTime(t, "2025-05-03T10:00:00Z")
	if !got.Equal(want) {
		t.Fatalf("expected next day %s, got %s", want, got)
	}
}

func TestParse_Errors(t *testing.T) {
	t.Parallel()
	bad := []string{
		"",
		"* * *",         // too few fields
		"* * * * * *",   // too many fields
		"60 * * * *",    // minute out of range
		"* 24 * * *",    // hour out of range
		"* * 0 * *",     // dom below min
		"* * * 13 *",    // month above max
		"* * * * 9",     // dow above 7
		"5-1 * * * *",   // inverted range
		"*/0 * * * *",   // zero step
		"*/-1 * * * *",  // negative step
		",* * * * *",    // empty list entry
		"@nopelopelope", // unknown descriptor
		"abc * * * *",   // non-numeric
		"1- * * * *",    // bad range
	}
	for _, expr := range bad {
		_, err := parseExpression(expr)
		if err == nil {
			t.Errorf("expected error for %q", expr)
		}
	}
}

func TestNext_TwoWildcards(t *testing.T) {
	t.Parallel()
	// When both DOM and DOW are stars, the schedule should fire on every
	// day matching the other restrictions.
	got, err := Next("30 9 * * *", mustTime(t, "2025-05-02T09:00:00Z"))
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	want := mustTime(t, "2025-05-02T09:30:00Z")
	if !got.Equal(want) {
		t.Fatalf("got %s, want %s", got, want)
	}
}
