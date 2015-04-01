package format

import (
	"fmt"
	"math"
	"time"
)

const (
	sizeB  = 1
	sizeKB = 1024 * sizeB
	sizeMB = 1024 * sizeKB
	sizeGB = 1024 * sizeMB
	sizeTB = 1024 * sizeGB
)

func Bytes(bytes float64) string {
	switch {
	case bytes < sizeKB:
		return fmt.Sprintf("%.2fB", bytes)
	case bytes < sizeMB:
		return fmt.Sprintf("%.2fKB", bytes/sizeKB)
	case bytes < sizeGB:
		return fmt.Sprintf("%.2fMB", bytes/sizeMB)
	case bytes < sizeTB:
		return fmt.Sprintf("%.2fGB", bytes/sizeGB)
	default:
		return fmt.Sprintf("%.2fTB", bytes/sizeTB)
	}
}

// TODO: tests for this method
func roundDuration(x time.Duration, precision float64) time.Duration {
	mod := math.Mod(float64(x), precision)
	if mod >= precision/2 {
		return x - time.Duration(mod)
	} else {
		return x - time.Duration(mod) + time.Duration(precision)
	}
}

func Reqps(r float64) string {
	switch {
	case r > 100:
		return fmt.Sprintf("%.0f", r)
	case r > 10:
		return fmt.Sprintf("%.1f", r)
	default:
		return fmt.Sprintf("%.2f", r)
	}
}

func Duration(d time.Duration) string {
	switch {
	case d > time.Hour:
		d = roundDuration(d, float64(time.Hour)/100)
	case d > time.Minute:
		d = roundDuration(d, float64(time.Minute)/100)
	case d > time.Second:
		d = roundDuration(d, float64(time.Second)/100)
	case d > time.Millisecond:
		d = roundDuration(d, float64(time.Millisecond)/100)
	case d > time.Microsecond:
		d = roundDuration(d, float64(time.Microsecond)/100)
	}

	return fmt.Sprintf("%v", d)
}
