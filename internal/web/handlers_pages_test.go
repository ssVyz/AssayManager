package web

import "testing"

func TestMmClass(t *testing.T) {
	cases := []struct {
		name         string
		pct          float64
		green, warn  float64
		higherBetter bool
		want         string
	}{
		// Higher-is-better (0 mm / ≤1 mm): green is the high end. Boundaries are
		// inclusive (≥).
		{"good high, at green", 90, 90, 70, true, "mm-good"},
		{"good high, above green", 99.5, 90, 70, true, "mm-good"},
		{"good high, in warn band", 80, 90, 70, true, "mm-warn"},
		{"good high, at warn", 70, 90, 70, true, "mm-warn"},
		{"good high, below warn", 69.9, 90, 70, true, "mm-bad"},
		{"good high, zero", 0, 90, 70, true, "mm-bad"},

		// Higher-is-worse (>1 mm): green is the low end. Boundaries inclusive (≤).
		{"bad high, at green", 5, 5, 20, false, "mm-good"},
		{"bad high, below green", 0, 5, 20, false, "mm-good"},
		{"bad high, in warn band", 12, 5, 20, false, "mm-warn"},
		{"bad high, at warn", 20, 5, 20, false, "mm-warn"},
		{"bad high, above warn", 20.1, 5, 20, false, "mm-bad"},
		{"bad high, hundred", 100, 5, 20, false, "mm-bad"},
	}
	for _, tc := range cases {
		if got := mmClass(tc.pct, tc.green, tc.warn, tc.higherBetter); got != tc.want {
			t.Errorf("%s: mmClass(%g, %g, %g, %v) = %q, want %q",
				tc.name, tc.pct, tc.green, tc.warn, tc.higherBetter, got, tc.want)
		}
	}
}
