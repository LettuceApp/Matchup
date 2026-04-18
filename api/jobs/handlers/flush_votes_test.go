package handlers

import "testing"

func TestParseVoteItemID(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		wantID uint
		wantOK bool
	}{
		{
			name:   "valid key",
			key:    "votes:item:42",
			wantID: 42,
			wantOK: true,
		},
		{
			name:   "zero id",
			key:    "votes:item:0",
			wantID: 0,
			wantOK: true,
		},
		{
			name:   "non-numeric tail",
			key:    "votes:item:abc",
			wantID: 0,
			wantOK: false,
		},
		{
			name:   "wrong prefix",
			key:    "wrong:prefix:42",
			wantID: 0,
			wantOK: false,
		},
		{
			name:   "empty string",
			key:    "",
			wantID: 0,
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotOK := parseVoteItemID(tt.key)
			if gotID != tt.wantID || gotOK != tt.wantOK {
				t.Errorf("parseVoteItemID(%q) = (%d, %v), want (%d, %v)",
					tt.key, gotID, gotOK, tt.wantID, tt.wantOK)
			}
		})
	}
}
