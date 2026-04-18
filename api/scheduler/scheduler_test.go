package scheduler

import (
	"testing"
	"time"
)

func TestAddMonths(t *testing.T) {
	tests := []struct {
		name string
		d    time.Time
		n    int
		want time.Time
	}{
		{
			name: "forward within same year",
			d:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			n:    3,
			want: time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "forward across year boundary",
			d:    time.Date(2024, 11, 1, 0, 0, 0, 0, time.UTC),
			n:    3,
			want: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "backward within same year",
			d:    time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
			n:    -1,
			want: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "backward across year boundary",
			d:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			n:    -1,
			want: time.Date(2023, 12, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "result always pinned to day 1",
			d:    time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			n:    2,
			want: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := addMonths(tt.d, tt.n)
			if !got.Equal(tt.want) {
				t.Errorf("addMonths(%v, %d) = %v, want %v", tt.d, tt.n, got, tt.want)
			}
			if got.Location() != time.UTC {
				t.Errorf("addMonths result not in UTC, got %v", got.Location())
			}
		})
	}
}
