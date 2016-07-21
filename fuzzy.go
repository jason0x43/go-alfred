package alfred

import (
	"math"
	"strings"
)

// FuzzyMatches returns true if val and test have a fuzzy match score != -1
func FuzzyMatches(val string, test string) bool {
	return fuzzyScore(val, test) >= 0
}

// fuzzyScore gives a score for how well the test script fuzzy matches a
// given value. To match, the test string must be equal to, or its characters
// must be an ordered subset of, the characters in the val string. A score of 0
// is a perfect match. Higher scores are lower quality matches. A score < 0
// indicates no match.
func fuzzyScore(val string, test string) float64 {
	if test == "" {
		return 0
	}

	lval := strings.ToLower(val)
	ltest := strings.ToLower(test)

	start := strings.IndexRune(lval, rune(ltest[0]))
	if start == -1 {
		return -1.0
	}
	start++

	startScore := 1 - (float64(start) / float64(len(lval)))

	score := 0.20 * startScore

	totalSep := 0
	i := 0

	for _, c := range ltest[1:] {
		if i = strings.IndexRune(lval[start:], c); i == -1 {
			return -1
		}
		totalSep += i
		start += i + 1
	}

	sepScore := math.Max(1-(float64(totalSep)/float64(len(test))), 0)

	score += 0.5 * sepScore

	matchScore := float64(len(test)) / float64(len(val))

	score += 0.2 * matchScore

	return score
}
