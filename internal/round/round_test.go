package round

import (
	"math"
	"testing"
)

func TestTo(t *testing.T) {
	// Valores esperados conferidos com round(x, n) do Python 3.12.
	tests := []struct {
		x      float64
		digits int
		want   float64
	}{
		{2.675, 2, 2.67},
		{0.125, 2, 0.12},
		{0.375, 2, 0.38},
		{2915.2463, 1, 2915.2},
		{0.80645, 3, 0.806},
		{0.8065, 3, 0.806},
		{1.0, 3, 1},
	}
	for _, tt := range tests {
		if got := To(tt.x, tt.digits); got != tt.want {
			t.Errorf("To(%v, %d) = %v, want %v", tt.x, tt.digits, got, tt.want)
		}
	}
	if got := To(math.Inf(1), 2); !math.IsInf(got, 1) {
		t.Errorf("To(+Inf) = %v", got)
	}
}
