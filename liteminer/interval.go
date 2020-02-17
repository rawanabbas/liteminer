/*
 *  Brown University, CS138, Spring 2020
 *
 *  Purpose: contains all interval related logic.
 */

package liteminer

// Interval represents [Lower, Upper)
type Interval struct {
	Lower uint64 // Inclusive
	Upper uint64 // Exclusive
}

// GenerateIntervals divides the range [0, upperBound] into numIntervals intervals.
func GenerateIntervals(upperBound uint64, numIntervals int) (intervals []Interval) {

	if (upperBound + 1) < uint64(numIntervals) {
		intervals = append(intervals, Interval{Lower: 0, Upper: upperBound + 1})
		return
	}

	var i uint64
	step := (upperBound + 1) / uint64(numIntervals)
	var lower, upper uint64

	for i = 0; i <= upperBound; i += step {
		lower = i
		if (i + step) > upperBound {
			upper = upperBound + 1
		} else {
			upper = i + step
		}

		if len(intervals) == (numIntervals - 1) {
			upper = upperBound + 1
			i = upper
		}

		intervals = append(intervals, Interval{Lower: lower, Upper: upper})
	}
	return
}

//GenerateIntervalsBySize takes the size of the interval and generate the apporiate numIntervals
func GenerateIntervalsBySize(upperBound uint64, size uint64) (intervals []Interval) {
	if (upperBound + 1) < uint64(size) {
		intervals = append(intervals, Interval{Lower: 0, Upper: upperBound + 1})
		return
	}
	numIntervals := (upperBound + 1) / size
	return GenerateIntervals(upperBound, int(numIntervals))
}
