package dedup

import (
	"testing"

	"github.com/helixir/literature-review-service/internal/domain"
)

func TestNormalizeName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple lowercase",
			input:    "John Smith",
			expected: "john smith",
		},
		{
			name:     "extra whitespace",
			input:    "  John   Smith  ",
			expected: "john smith",
		},
		{
			name:     "last comma first format",
			input:    "SMITH, John",
			expected: "john smith",
		},
		{
			name:     "apostrophe removed",
			input:    "O'Brien",
			expected: "obrien",
		},
		{
			name:     "periods removed",
			input:    "J. K. Rowling",
			expected: "j k rowling",
		},
		{
			name:     "hyphens removed",
			input:    "Mary-Jane Watson",
			expected: "maryjane watson",
		},
		{
			name:     "all caps last comma first",
			input:    "DOE, Jane",
			expected: "jane doe",
		},
		{
			name:     "already normalized",
			input:    "john smith",
			expected: "john smith",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    "   ",
			expected: "",
		},
		{
			name:     "unicode accented characters preserved",
			input:    "Jose Garcia",
			expected: "jose garcia",
		},
		{
			name:     "last comma first with extra spaces",
			input:    "  Smith ,  John  ",
			expected: "john smith",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeName(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNameSimilarity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		a        string
		b        string
		expected float64
	}{
		{
			name:     "exact match",
			a:        "john smith",
			b:        "john smith",
			expected: 1.0,
		},
		{
			name:     "same last same first",
			a:        "john smith",
			b:        "john smith",
			expected: 1.0,
		},
		{
			name:     "same last initial match",
			a:        "j smith",
			b:        "john smith",
			expected: 0.9,
		},
		{
			name:     "same last initial match reversed",
			a:        "john smith",
			b:        "j smith",
			expected: 0.9,
		},
		{
			name:     "same last only last available",
			a:        "smith",
			b:        "smith",
			expected: 0.7,
		},
		{
			name:     "same last different first",
			a:        "john smith",
			b:        "jane smith",
			expected: 0.3,
		},
		{
			name:     "completely different",
			a:        "john smith",
			b:        "alice johnson",
			expected: 0.0,
		},
		{
			name:     "empty strings",
			a:        "",
			b:        "",
			expected: 0.0,
		},
		{
			name:     "one empty",
			a:        "john smith",
			b:        "",
			expected: 0.0,
		},
		{
			name:     "single initial vs full first name same last",
			a:        "j smith",
			b:        "john smith",
			expected: 0.9,
		},
		{
			name:     "one has only last name other has full name same last",
			a:        "smith",
			b:        "john smith",
			expected: 0.7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := nameSimilarity(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("nameSimilarity(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.expected)
			}
		})
	}
}

func TestAuthorOverlap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		a       []domain.Author
		b       []domain.Author
		wantMin float64
		wantMax float64
	}{
		{
			name:    "both empty",
			a:       nil,
			b:       nil,
			wantMin: 0.0,
			wantMax: 0.0,
		},
		{
			name:    "first empty",
			a:       nil,
			b:       []domain.Author{{Name: "John Smith"}},
			wantMin: 0.0,
			wantMax: 0.0,
		},
		{
			name:    "second empty",
			a:       []domain.Author{{Name: "John Smith"}},
			b:       nil,
			wantMin: 0.0,
			wantMax: 0.0,
		},
		{
			name:    "exact match single author",
			a:       []domain.Author{{Name: "John Smith"}},
			b:       []domain.Author{{Name: "John Smith"}},
			wantMin: 1.0,
			wantMax: 1.0,
		},
		{
			name: "exact match multiple authors",
			a: []domain.Author{
				{Name: "John Smith"},
				{Name: "Jane Doe"},
			},
			b: []domain.Author{
				{Name: "John Smith"},
				{Name: "Jane Doe"},
			},
			wantMin: 1.0,
			wantMax: 1.0,
		},
		{
			name: "exact match different order",
			a: []domain.Author{
				{Name: "Jane Doe"},
				{Name: "John Smith"},
			},
			b: []domain.Author{
				{Name: "John Smith"},
				{Name: "Jane Doe"},
			},
			wantMin: 1.0,
			wantMax: 1.0,
		},
		{
			name: "no overlap completely different",
			a: []domain.Author{
				{Name: "John Smith"},
			},
			b: []domain.Author{
				{Name: "Alice Johnson"},
			},
			wantMin: 0.0,
			wantMax: 0.0,
		},
		{
			name: "partial match with abbreviation",
			a: []domain.Author{
				{Name: "J. Smith"},
				{Name: "Jane Doe"},
			},
			b: []domain.Author{
				{Name: "John Smith"},
				{Name: "Jane Doe"},
			},
			wantMin: 0.5,
			wantMax: 1.0,
		},
		{
			name: "superset overlap 3 vs 2 shared",
			a: []domain.Author{
				{Name: "John Smith"},
				{Name: "Jane Doe"},
				{Name: "Alice Johnson"},
			},
			b: []domain.Author{
				{Name: "John Smith"},
				{Name: "Jane Doe"},
			},
			wantMin: 0.6,
			wantMax: 1.0,
		},
		{
			name: "last comma first format match",
			a: []domain.Author{
				{Name: "Smith, John"},
			},
			b: []domain.Author{
				{Name: "John Smith"},
			},
			wantMin: 1.0,
			wantMax: 1.0,
		},
		{
			name: "case insensitive match",
			a: []domain.Author{
				{Name: "JOHN SMITH"},
			},
			b: []domain.Author{
				{Name: "john smith"},
			},
			wantMin: 1.0,
			wantMax: 1.0,
		},
		{
			name: "no overlap multiple authors",
			a: []domain.Author{
				{Name: "John Smith"},
				{Name: "Jane Doe"},
			},
			b: []domain.Author{
				{Name: "Alice Johnson"},
				{Name: "Bob Williams"},
			},
			wantMin: 0.0,
			wantMax: 0.0,
		},
		{
			name: "one shared one different",
			a: []domain.Author{
				{Name: "John Smith"},
				{Name: "Jane Doe"},
			},
			b: []domain.Author{
				{Name: "John Smith"},
				{Name: "Alice Johnson"},
			},
			wantMin: 0.3,
			wantMax: 0.6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := AuthorOverlap(tt.a, tt.b)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("AuthorOverlap() = %v, want between %v and %v", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestAuthorOverlap_Symmetry(t *testing.T) {
	t.Parallel()

	a := []domain.Author{
		{Name: "John Smith"},
		{Name: "Jane Doe"},
		{Name: "Alice Johnson"},
	}
	b := []domain.Author{
		{Name: "John Smith"},
		{Name: "Jane Doe"},
	}

	ab := AuthorOverlap(a, b)
	ba := AuthorOverlap(b, a)

	if ab != ba {
		t.Errorf("AuthorOverlap is not symmetric: (%v, %v) = %v, (%v, %v) = %v",
			a, b, ab, b, a, ba)
	}
}

func TestNormalizeAuthors(t *testing.T) {
	t.Parallel()

	authors := []domain.Author{
		{Name: "SMITH, John"},
		{Name: "J. Doe"},
		{Name: "Alice O'Brien"},
	}

	got := normalizeAuthors(authors)
	expected := []string{
		"john smith",
		"j doe",
		"alice obrien",
	}

	if len(got) != len(expected) {
		t.Fatalf("normalizeAuthors() returned %d items, want %d", len(got), len(expected))
	}

	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("normalizeAuthors()[%d] = %q, want %q", i, got[i], expected[i])
		}
	}
}
