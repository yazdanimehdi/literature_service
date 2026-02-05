// Package dedup provides utilities for detecting duplicate academic papers
// through fuzzy matching of author lists and name normalization.
package dedup

import (
	"strings"
	"unicode"

	"github.com/helixir/literature-review-service/internal/domain"
)

// AuthorOverlap computes a fuzzy overlap score between two author lists.
// It uses best-match pairing: each author in the smaller list is matched to
// the most similar unmatched author in the larger list, then computes a
// Jaccard-style score by dividing the total matched similarity by the union count.
//
// Returns 0.0 if either list is empty, 1.0 for a perfect match.
// The result is symmetric: AuthorOverlap(a, b) == AuthorOverlap(b, a).
func AuthorOverlap(a, b []domain.Author) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}

	normA := normalizeAuthors(a)
	normB := normalizeAuthors(b)

	// Ensure normA is the smaller list for pairing.
	if len(normA) > len(normB) {
		normA, normB = normB, normA
	}

	// Best-match pairing: for each name in the smaller list, find the most
	// similar unmatched name in the larger list using a greedy approach.
	used := make([]bool, len(normB))
	totalScore := 0.0

	for _, nameA := range normA {
		bestScore := 0.0
		bestIdx := -1

		for j, nameB := range normB {
			if used[j] {
				continue
			}
			score := nameSimilarity(nameA, nameB)
			if score > bestScore {
				bestScore = score
				bestIdx = j
			}
		}

		if bestIdx >= 0 {
			used[bestIdx] = true
			totalScore += bestScore
		}
	}

	// Jaccard-style: matched score divided by union count.
	// Union count = |A| + |B| - |matched pairs|
	matchedPairs := 0
	for _, u := range used {
		if u {
			matchedPairs++
		}
	}
	unionCount := len(normA) + len(normB) - matchedPairs

	if unionCount == 0 {
		return 0.0
	}

	return totalScore / float64(unionCount)
}

// NormalizeName normalizes an author name for comparison:
//   - Converts to lowercase
//   - Detects and reorders "Last, First" format to "First Last"
//   - Removes all non-letter, non-space characters (apostrophes, periods, hyphens, etc.)
//   - Collapses multiple spaces to a single space
//   - Trims leading and trailing whitespace
func NormalizeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	// Convert to lowercase first.
	name = strings.ToLower(name)

	// Handle "Last, First" format: split on comma, swap parts.
	if idx := strings.Index(name, ","); idx >= 0 {
		last := strings.TrimSpace(name[:idx])
		first := strings.TrimSpace(name[idx+1:])
		if first != "" {
			name = first + " " + last
		} else {
			name = last
		}
	}

	// Remove non-letter, non-space characters.
	var sb strings.Builder
	sb.Grow(len(name))
	prevSpace := false

	for _, r := range name {
		if unicode.IsLetter(r) {
			sb.WriteRune(r)
			prevSpace = false
		} else if unicode.IsSpace(r) {
			if !prevSpace && sb.Len() > 0 {
				sb.WriteRune(' ')
				prevSpace = true
			}
		}
		// All other characters (apostrophes, periods, hyphens) are dropped.
	}

	result := sb.String()
	return strings.TrimRight(result, " ")
}

// nameSimilarity compares two normalized author names and returns a similarity
// score between 0.0 and 1.0.
//
// Scoring rules:
//   - Exact match: 1.0
//   - Same last name, same first name: 1.0
//   - Same last name, one first name is an initial that matches: 0.9
//   - Same last name, one or both have only a last name: 0.7
//   - Same last name, different first names: 0.3
//   - Different last names: 0.0
func nameSimilarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0.0
	}

	partsA := strings.Fields(a)
	partsB := strings.Fields(b)

	// Extract last name (last token) and first name parts (everything before).
	lastA := partsA[len(partsA)-1]
	lastB := partsB[len(partsB)-1]

	if lastA != lastB {
		return 0.0
	}

	// Same last name -- compare first name parts.
	firstA := partsA[:len(partsA)-1]
	firstB := partsB[:len(partsB)-1]

	// If either has no first name, return 0.7 (last-name-only match).
	if len(firstA) == 0 || len(firstB) == 0 {
		return 0.7
	}

	// Compare first name tokens. Use the first token of each for primary comparison.
	fA := strings.Join(firstA, " ")
	fB := strings.Join(firstB, " ")

	// Exact first name match.
	if fA == fB {
		return 1.0
	}

	// Check if one is an initial of the other.
	// An initial is a single character that matches the first character of the other name.
	firstTokenA := firstA[0]
	firstTokenB := firstB[0]

	if isInitialMatch(firstTokenA, firstTokenB) {
		return 0.9
	}

	// Same last name but different first names.
	return 0.3
}

// isInitialMatch returns true if one token is a single-character initial that
// matches the first character of the other token.
func isInitialMatch(a, b string) bool {
	if len(a) == 1 && len(b) > 1 && a[0] == b[0] {
		return true
	}
	if len(b) == 1 && len(a) > 1 && b[0] == a[0] {
		return true
	}
	return false
}

// normalizeAuthors applies NormalizeName to each author's Name field and
// returns the resulting slice of normalized name strings.
func normalizeAuthors(authors []domain.Author) []string {
	result := make([]string, len(authors))
	for i, a := range authors {
		result[i] = NormalizeName(a.Name)
	}
	return result
}
