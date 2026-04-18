package controllers

import "testing"

func TestVoteDeltaKey(t *testing.T) {
	tests := []struct {
		name   string
		itemID uint
		want   string
	}{
		{
			name:   "typical id",
			itemID: 42,
			want:   "votes:item:42",
		},
		{
			name:   "zero id",
			itemID: 0,
			want:   "votes:item:0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := voteDeltaKey(tt.itemID)
			if got != tt.want {
				t.Errorf("voteDeltaKey(%d) = %q, want %q", tt.itemID, got, tt.want)
			}
		})
	}
}

func TestVoteDeltaKeyPrefix(t *testing.T) {
	want := "votes:item:"
	if VoteDeltaKeyPrefix != want {
		t.Errorf("VoteDeltaKeyPrefix = %q, want %q", VoteDeltaKeyPrefix, want)
	}
}
