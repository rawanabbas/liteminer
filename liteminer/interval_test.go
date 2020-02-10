package liteminer

import "testing"

func TestGenerateIntervals(t *testing.T) {
	intervals := GenerateIntervals(100, 8)

	if len(intervals) != 8 {
		t.Errorf("Mismatched number of intervals should be %v but the number is %v", 8, len(intervals))
	}

	if intervals[len(intervals)-1].Upper != 101 {
		t.Errorf("Intervals doesn't reach or exceeds the upperBound should be %v but found %v\n", 100, intervals[len(intervals)-1])
	}

	t.Log("Success!")
}
