package ranking

import (
	"testing"
	"time"
)

func TestParseWindowAndShanghaiCalendarStart(t *testing.T) {
	if _, err := ParseWindow("month"); err == nil {
		t.Fatal("month should be rejected")
	}
	instant := time.Date(2026, 9, 23, 0, 30, 0, 0, time.UTC)
	day := WindowDay.start(instant)
	if got := day.Format("2006-01-02 15:04:05 -0700"); got != "2026-09-23 00:00:00 +0800" {
		t.Fatalf("day start=%s", got)
	}
	week := WindowWeek.start(instant)
	if got := week.Format("2006-01-02"); got != "2026-09-17" {
		t.Fatalf("week start=%s", got)
	}
}

func TestScoreNeverNegativeAndMemberTieOrder(t *testing.T) {
	if got := Score(10, 999, 999, 2, 3, 4); got != 39 {
		t.Fatalf("score=%v, want 39", got)
	}
	if got := Score(0, 0, 0, -100, -100, -100); got != 0 {
		t.Fatalf("negative score=%v", got)
	}
	if encodeMember(9) >= encodeMember(10) {
		t.Fatalf("member encoding does not sort higher ids first: 9=%q 10=%q", encodeMember(9), encodeMember(10))
	}
	for _, id := range []uint64{1, 9, 10, ^uint64(0)} {
		decoded, err := decodeMember(encodeMember(id))
		if err != nil || decoded != id {
			t.Fatalf("round trip id=%d decoded=%d err=%v", id, decoded, err)
		}
	}
}
