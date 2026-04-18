package jobs

import "testing"

func TestQueueKey(t *testing.T) {
	got := queueKey("email")
	want := "matchup:jobs:email"
	if got != want {
		t.Errorf("queueKey(%q) = %q, want %q", "email", got, want)
	}
}

func TestDlqKey(t *testing.T) {
	got := dlqKey("email")
	want := "matchup:jobs:email:dlq"
	if got != want {
		t.Errorf("dlqKey(%q) = %q, want %q", "email", got, want)
	}
}

func TestMaxQueueLen(t *testing.T) {
	if MaxQueueLen != 10000 {
		t.Errorf("MaxQueueLen = %d, want 10000", MaxQueueLen)
	}
}

func TestMaxDLQLen(t *testing.T) {
	if MaxDLQLen != 1000 {
		t.Errorf("MaxDLQLen = %d, want 1000", MaxDLQLen)
	}
}
