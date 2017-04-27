package alfred

import (
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
	// A blank string matches anything
	if test == "" {
		return 0
	}

	lval := strings.ToLower(val)
	ltest := strings.ToLower(test)

	start := strings.IndexRune(lval, rune(ltest[0]))
	if start == -1 {
		return -1.0
	}

	// The score component based on how far into val the test string starts. If
	// the test string starts on the first character of val, this will be 0.
	startScore := 1.0 - (float64(len(lval)-start) / float64(len(lval)))
	score := 0.40 * startScore

	end := start

	for _, c := range ltest[1:] {
		// Return a non-match if the next character isn't in the string
		if i := strings.IndexRune(lval[end:], c); i == -1 {
			return -1
		} else {
			end += i + 1
		}
	}

	// The score component based on how far spread out the matching characters
	// are. If the characters are contiguous, this will be 0.
	sizeDelta := len(val) - len(test)
	sepScore := float64((end-start)-len(test)) / float64(sizeDelta)
	score += 0.4 * sepScore

	// The score component based on the ratio of test string length to the val string length
	matchScore := 1.0 - (float64(len(test)) / float64(len(val)))
	score += 0.2 * matchScore

	// dlog.Print("Score for ", val, ": ", score)
	// dlog.Print("  start score: ", startScore)
	// dlog.Print("  sep score: ", sepScore, " (", start, ", ", end, ")")
	// dlog.Print("  match score: ", matchScore)

	return score
}
