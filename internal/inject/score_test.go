package inject

import "testing"

func TestInjectionScoreBoundaries(t *testing.T) {
	for _, c := range []struct{ n, w int }{{0, 10}, {49, 10}, {50, 8}, {999, 8}, {1000, 8}, {1001, 6}, {3000, 6}, {3001, 4}, {6000, 4}, {6001, 2}, {10000, 2}, {10001, 0}} {
		if got := InjectionScore(c.n); got != c.w {
			t.Errorf("%d: %d != %d", c.n, got, c.w)
		}
	}
}
