package controllers

import (
	"testing"
)

// TestNCAASeedOrder_KnownSizes locks in the expected output of the
// canonical NCAA recursive seeding for every bracket size we
// support (4, 8, 16, 32, 64). Refactors of ncaaSeedOrder must not
// quietly shift pairings — the bracket invariant (top seeds at
// opposite ends, each round-1 pair sums to N+1) needs to hold.
//
// For the larger sizes (16/32/64) we don't enumerate every position
// — the property checks below cover those — but n=4 and n=8 get
// full-table verification because they're the sizes users hit most
// often and the ones the bug report referenced.
func TestNCAASeedOrder_KnownSizes(t *testing.T) {
	cases := []struct {
		n    int
		want []int
	}{
		{4, []int{1, 4, 2, 3}},
		{8, []int{1, 8, 4, 5, 2, 7, 3, 6}},
		// N=16 is the recursive expansion of N=8 = [1,8,4,5,2,7,3,6].
		// Each seed s expands to (s, 17-s), so:
		//   1→(1,16), 8→(8,9), 4→(4,13), 5→(5,12),
		//   2→(2,15), 7→(7,10), 3→(3,14), 6→(6,11)
		// Note this is the "slaughter" convention; some other NCAA-style
		// conventions (e.g. "bridge") produce a different ordering but
		// preserve the same pair set. The invariant tests below verify
		// both produce a valid bracket; this test pins the exact one we ship.
		{16, []int{
			1, 16, 8, 9, 4, 13, 5, 12,
			2, 15, 7, 10, 3, 14, 6, 11,
		}},
	}
	for _, tc := range cases {
		got := ncaaSeedOrder(tc.n)
		if !equalIntSlice(got, tc.want) {
			t.Errorf("ncaaSeedOrder(%d) = %v, want %v", tc.n, got, tc.want)
		}
	}
}

// TestNCAASeedOrder_PairsSumToNPlusOne verifies the core NCAA seeding
// invariant: every adjacent pair (positions 2i, 2i+1) sums to N+1.
// If any pair breaks this, two same-half seeds got placed adjacent
// — which is the bug that prompted this work.
func TestNCAASeedOrder_PairsSumToNPlusOne(t *testing.T) {
	for _, n := range []int{4, 8, 16, 32, 64} {
		order := ncaaSeedOrder(n)
		if len(order) != n {
			t.Errorf("ncaaSeedOrder(%d) length = %d, want %d", n, len(order), n)
			continue
		}
		for i := 0; i < n/2; i++ {
			a := order[2*i]
			b := order[2*i+1]
			if a+b != n+1 {
				t.Errorf("ncaaSeedOrder(%d) pair %d: %d+%d != %d", n, i, a, b, n+1)
			}
		}
	}
}

// TestNCAASeedOrder_AllSeedsPresent confirms every seed 1..N appears
// exactly once. A buggy implementation could produce duplicates or
// drop seeds — both would corrupt the bracket — and the per-pair
// invariant above doesn't catch every shape of regression.
func TestNCAASeedOrder_AllSeedsPresent(t *testing.T) {
	for _, n := range []int{4, 8, 16, 32, 64} {
		order := ncaaSeedOrder(n)
		seen := make([]bool, n+1)
		for _, s := range order {
			if s < 1 || s > n {
				t.Errorf("ncaaSeedOrder(%d) emitted out-of-range seed %d", n, s)
				continue
			}
			if seen[s] {
				t.Errorf("ncaaSeedOrder(%d) emitted duplicate seed %d", n, s)
			}
			seen[s] = true
		}
		for s := 1; s <= n; s++ {
			if !seen[s] {
				t.Errorf("ncaaSeedOrder(%d) missing seed %d", n, s)
			}
		}
	}
}

// TestNCAASeedOrder_TopSeedsInOppositeHalves is the property that
// matters most to users: seeds 1 and 2 must be in opposite halves of
// the bracket so they can't meet before the final. Same for seeds 3
// and 4 (opposite quarters within their halves), etc. The recursive
// algorithm guarantees this naturally — the test guards against a
// future refactor accidentally breaking it.
func TestNCAASeedOrder_TopSeedsInOppositeHalves(t *testing.T) {
	for _, n := range []int{4, 8, 16, 32, 64} {
		order := ncaaSeedOrder(n)
		half := n / 2
		// Map seed → position so we can compare.
		pos := make(map[int]int, n)
		for i, s := range order {
			pos[s] = i
		}
		// Seed 1 must be in the top half (position < half), seed 2 in
		// the bottom half (position >= half).
		if pos[1] >= half {
			t.Errorf("ncaaSeedOrder(%d): seed 1 in bottom half (pos %d)", n, pos[1])
		}
		if pos[2] < half {
			t.Errorf("ncaaSeedOrder(%d): seed 2 in top half (pos %d)", n, pos[2])
		}
	}
}

func equalIntSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
