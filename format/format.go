package format

import (
	"fmt"
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
		return fmt.Sprintf("%.2fKB", bytes/sizeMB)
	case bytes < sizeTB:
		return fmt.Sprintf("%.2fKB", bytes/sizeGB)
	default:
		return fmt.Sprintf("%.2fKB", bytes/sizeTB)
	}
}
