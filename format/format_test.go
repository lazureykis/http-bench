package format

import (
	"testing"
	"time"
)

func TestRoundDuration(t *testing.T) {
	cases := [][]int{
		[]int{65865865, 10000, 65870000},
		[]int{65865000, 10000, 65870000},
		[]int{65864999, 10000, 65860000},
		[]int{3, 2, 4},
	}

	for _, v := range cases {
		want := time.Duration(v[2])
		got := RoundDuration(time.Duration(v[0]), float64(v[1]))
		if got != want {
			t.Errorf("Want %v, got %v", want, got)
		}
	}
}
