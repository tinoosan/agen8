package main

import (
	"fmt"
	"math"
)

const (
	minLeftWidth  = 60
	minRightWidth = 32
	gapCols       = 1
)

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func calculateDashboard(width int) (leftW, rightW int) {
	minTotalWidth := minLeftWidth + minRightWidth + gapCols

	if width < minTotalWidth {
		available := width - gapCols
		if available < 0 {
			available = 0
		}
		leftW = int(math.Round(float64(available) * 0.66))
		rightW = available - leftW
		if leftW < 1 && available >= 2 {
			leftW = 1
			rightW = available - 1
		} else if rightW < 1 && available >= 2 {
			rightW = 1
			leftW = available - 1
		}
	} else {
		leftW = int(math.Round(float64(width) * 0.66))
		if leftW < minLeftWidth {
			leftW = minLeftWidth
		}
		if leftW > width-minRightWidth-gapCols {
			leftW = max(0, width-minRightWidth-gapCols)
		}
		rightW = width - leftW - gapCols
		if rightW < minRightWidth {
			rightW = minRightWidth
			leftW = max(0, width-rightW-gapCols)
		}
	}
	return
}

func main() {
	fmt.Println("Testing widths 0 to 150...")
	found := false
	for w := 0; w <= 150; w++ {
		l, r := calculateDashboard(w)
		total := l + r + gapCols
		if total > w {
			// Special case: if w is very small, gap might make total > w?
			// But calculateDashboard logic should ensure l+r fits in (w-gap).
			// If w < gapCols, then l=0, r=0, total = gapCols = 1.
			// If w=0, total=1 > 0. This is an overflow case for 0 width, but terminal usually guarantees >=1.
			if w > 0 {
				fmt.Printf("OVERFLOW: width=%d -> left=%d right=%d total=%d\n", w, l, r, total)
				found = true
			}
		}
	}
	if !found {
		fmt.Println("No overflows found for widths > 0.")
	}
}
