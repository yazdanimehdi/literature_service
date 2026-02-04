package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// PaperIdentifiers holds all possible identifiers for an academic paper.
type PaperIdentifiers struct {
	DOI               string
	ArXivID           string
	PubMedID          string
	PMCID             string
	SemanticScholarID string
	OpenAlexID        string
	ScopusID          string
}

// GenerateCanonicalID generates a canonical identifier from paper identifiers.
// Priority order: DOI > ArXiv > PubMed > SemanticScholar > OpenAlex > Scopus
// Returns empty string if no identifiers are available.
func GenerateCanonicalID(ids PaperIdentifiers) string {
	// Check DOI first (highest priority)
	if doi := strings.TrimSpace(ids.DOI); doi != "" {
		// Normalize DOI to lowercase
		return "doi:" + strings.ToLower(doi)
	}

	// ArXiv ID
	if arxiv := strings.TrimSpace(ids.ArXivID); arxiv != "" {
		return "arxiv:" + arxiv
	}

	// PubMed ID
	if pubmed := strings.TrimSpace(ids.PubMedID); pubmed != "" {
		return "pubmed:" + pubmed
	}

	// Semantic Scholar ID
	if s2 := strings.TrimSpace(ids.SemanticScholarID); s2 != "" {
		return "s2:" + s2
	}

	// OpenAlex ID
	if openalex := strings.TrimSpace(ids.OpenAlexID); openalex != "" {
		return "openalex:" + openalex
	}

	// Scopus ID (lowest priority)
	if scopus := strings.TrimSpace(ids.ScopusID); scopus != "" {
		return "scopus:" + scopus
	}

	// No identifier available
	return ""
}

// Author represents a paper author with optional affiliation and ORCID.
type Author struct {
	Name        string `json:"name"`
	Affiliation string `json:"affiliation,omitempty"`
	ORCID       string `json:"orcid,omitempty"`
}

// String returns a formatted string representation of the author.
func (a Author) String() string {
	var sb strings.Builder
	sb.WriteString(a.Name)

	if a.Affiliation != "" {
		sb.WriteString(" (")
		sb.WriteString(a.Affiliation)
		sb.WriteString(")")
	}

	if a.ORCID != "" {
		sb.WriteString(" [")
		sb.WriteString(a.ORCID)
		sb.WriteString("]")
	}

	return sb.String()
}

// Paper represents an academic paper in the central repository.
type Paper struct {
	ID                uuid.UUID
	CanonicalID       string
	Title             string
	Abstract          string
	Authors           []Author
	PublicationDate   *time.Time
	PublicationYear   int
	Venue             string
	Journal           string
	Volume            string
	Issue             string
	Pages             string
	CitationCount     int
	ReferenceCount    int
	PDFURL            string
	OpenAccess        bool
	KeywordsExtracted bool
	RawMetadata       map[string]interface{}
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// HasIdentifier returns true if the paper has at least one identifier.
func (p *Paper) HasIdentifier() bool {
	return p.CanonicalID != ""
}
