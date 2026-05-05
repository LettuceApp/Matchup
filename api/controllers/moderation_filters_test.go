package controllers

import "testing"

// Pure-Go tests for excludeUsersFragment. blockedUserIDs / mutedUserIDs
// / hiddenForViewer all hit Postgres; those are covered by the
// block/mute integration tests alongside the handler tests.

func TestExcludeUsersFragment_Empty(t *testing.T) {
	frag, args := excludeUsersFragment("u.id", nil, 1)
	if frag != "" {
		t.Errorf("expected empty fragment for nil ids, got %q", frag)
	}
	if args != nil {
		t.Errorf("expected nil args, got %v", args)
	}

	frag2, args2 := excludeUsersFragment("u.id", []uint{}, 1)
	if frag2 != "" || args2 != nil {
		t.Errorf("expected empty fragment for empty ids, got frag=%q args=%v", frag2, args2)
	}
}

func TestExcludeUsersFragment_NumbersPlaceholdersFromStart(t *testing.T) {
	// Caller already has $1 + $2 in their query; we start at $3.
	frag, args := excludeUsersFragment("u.id", []uint{42, 99, 7}, 3)
	want := " AND u.id NOT IN ($3, $4, $5)"
	if frag != want {
		t.Errorf("frag = %q, want %q", frag, want)
	}
	if len(args) != 3 || args[0] != uint(42) || args[1] != uint(99) || args[2] != uint(7) {
		t.Errorf("args = %v, want [42 99 7]", args)
	}
}

func TestExcludeUsersFragment_SingleUser(t *testing.T) {
	frag, args := excludeUsersFragment("blocker.id", []uint{1}, 1)
	want := " AND blocker.id NOT IN ($1)"
	if frag != want {
		t.Errorf("frag = %q, want %q", frag, want)
	}
	if len(args) != 1 || args[0] != uint(1) {
		t.Errorf("args = %v", args)
	}
}
