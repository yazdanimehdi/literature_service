package workflows

import "sort"

// SortedMapKeys returns the keys of a map sorted in ascending order.
// This is critical for Temporal workflow determinism — Go maps iterate
// in random order, so any workflow logic that iterates a map must sort
// keys first to ensure identical execution on replay.
func SortedMapKeys[K ~string, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
	return keys
}

// SortedStringSlice returns a sorted copy of a string slice without
// mutating the original. Use this before passing string slices into
// Temporal activities to ensure deterministic ordering.
func SortedStringSlice(s []string) []string {
	if len(s) == 0 {
		return []string{}
	}
	result := make([]string, len(s))
	copy(result, s)
	sort.Strings(result)
	return result
}

// DeduplicateStrings removes duplicate strings and returns the result
// sorted in ascending order. The input slice is not modified.
func DeduplicateStrings(s []string) []string {
	if len(s) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(s))
	result := make([]string, 0, len(s))
	for _, v := range s {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	sort.Strings(result)
	return result
}
