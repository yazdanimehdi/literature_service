package workflows

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSortedMapKeys(t *testing.T) {
	t.Run("returns keys in sorted order", func(t *testing.T) {
		m := map[string]int{"c": 3, "a": 1, "b": 2}
		keys := SortedMapKeys(m)
		assert.Equal(t, []string{"a", "b", "c"}, keys)
	})

	t.Run("empty map returns empty slice", func(t *testing.T) {
		m := map[string]int{}
		keys := SortedMapKeys(m)
		assert.Empty(t, keys)
	})
}

func TestSortedStringSlice(t *testing.T) {
	t.Run("returns sorted copy without mutating original", func(t *testing.T) {
		original := []string{"c", "a", "b"}
		sorted := SortedStringSlice(original)
		assert.Equal(t, []string{"a", "b", "c"}, sorted)
		assert.Equal(t, []string{"c", "a", "b"}, original)
	})

	t.Run("empty slice returns empty slice", func(t *testing.T) {
		sorted := SortedStringSlice([]string{})
		assert.Empty(t, sorted)
	})

	t.Run("nil slice returns empty slice", func(t *testing.T) {
		sorted := SortedStringSlice(nil)
		assert.Empty(t, sorted)
	})
}

func TestDeduplicateStrings(t *testing.T) {
	t.Run("removes duplicates and sorts", func(t *testing.T) {
		input := []string{"b", "a", "b", "c", "a"}
		result := DeduplicateStrings(input)
		assert.Equal(t, []string{"a", "b", "c"}, result)
	})

	t.Run("no duplicates returns sorted", func(t *testing.T) {
		input := []string{"c", "a", "b"}
		result := DeduplicateStrings(input)
		assert.Equal(t, []string{"a", "b", "c"}, result)
	})

	t.Run("empty returns empty", func(t *testing.T) {
		result := DeduplicateStrings(nil)
		assert.Empty(t, result)
	})
}
