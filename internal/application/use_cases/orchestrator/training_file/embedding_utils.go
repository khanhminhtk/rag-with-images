package trainingfile

import (
	"fmt"
	"math"
	"strings"
)

func vectorL2Norm(vec []float32) float64 {
	var sum float64
	for _, v := range vec {
		f := float64(v)
		sum += f * f
	}
	return math.Sqrt(sum)
}

func isZeroNormVector(vec []float32) bool {
	if len(vec) == 0 {
		return true
	}
	return vectorL2Norm(vec) < 1e-12
}

func formatEmbeddingProgressBar(processed, total, width int) string {
	if width <= 0 {
		width = 24
	}
	if total <= 0 {
		return "[" + strings.Repeat("-", width) + "] 0.0%"
	}
	if processed < 0 {
		processed = 0
	}
	if processed > total {
		processed = total
	}

	ratio := float64(processed) / float64(total)
	filled := int(math.Round(ratio * float64(width)))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}

	bar := "[" + strings.Repeat("#", filled) + strings.Repeat("-", width-filled) + "]"
	return fmt.Sprintf("%s %.1f%%", bar, ratio*100.0)
}
