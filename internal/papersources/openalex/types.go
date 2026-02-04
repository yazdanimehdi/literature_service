// Package openalex provides a client for the OpenAlex API.
//
// OpenAlex is a free, open catalog of scholarly papers, authors, venues,
// institutions, and concepts. This package implements the PaperSource
// interface for searching and retrieving academic papers from OpenAlex.
//
// API Documentation: https://docs.openalex.org/
package openalex

// SearchResponse represents the top-level response from the OpenAlex works search endpoint.
type SearchResponse struct {
	Meta    Meta   `json:"meta"`
	Results []Work `json:"results"`
}

// Meta contains metadata about the search results including pagination info.
type Meta struct {
	Count      int    `json:"count"`
	DBTime     int    `json:"db_response_time_ms"`
	Page       int    `json:"page"`
	PerPage    int    `json:"per_page"`
	NextCursor string `json:"next_cursor"`
}

// Work represents an academic work (paper) in OpenAlex.
type Work struct {
	ID              string       `json:"id"`
	DOI             string       `json:"doi"`
	Title           string       `json:"title"`
	DisplayName     string       `json:"display_name"`
	PublicationYear int          `json:"publication_year"`
	PublicationDate string       `json:"publication_date"`
	Type            string       `json:"type"`
	CitedByCount    int          `json:"cited_by_count"`
	IsOpenAccess    bool         `json:"is_oa"`
	OpenAccess      *OpenAccess  `json:"open_access"`
	Authorships     []Authorship `json:"authorships"`
	PrimaryLocation *Location    `json:"primary_location"`
	IDs             IDs          `json:"ids"`
	ReferencedWorks []string     `json:"referenced_works"`

	// Abstract is stored as an inverted index - we will reconstruct it
	AbstractInvertedIndex map[string][]int `json:"abstract_inverted_index"`
}

// OpenAccess contains open access information for a work.
type OpenAccess struct {
	IsOA     bool   `json:"is_oa"`
	OAURL    string `json:"oa_url"`
	OAStatus string `json:"oa_status"`
}

// Authorship represents an author's contribution to a work.
type Authorship struct {
	AuthorPosition string        `json:"author_position"`
	Author         AuthorInfo    `json:"author"`
	Institutions   []Institution `json:"institutions"`
}

// AuthorInfo contains basic author information.
type AuthorInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Orcid       string `json:"orcid"`
}

// Institution represents an academic institution.
type Institution struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// Location represents where a work is available.
type Location struct {
	Source  *Source `json:"source"`
	PDFURL  string  `json:"pdf_url"`
	Version string  `json:"version"`
}

// Source represents a publication venue (journal, repository, etc.).
type Source struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
}

// IDs contains various identifiers for a work.
type IDs struct {
	OpenAlex string `json:"openalex"`
	DOI      string `json:"doi"`
	MAG      string `json:"mag"`
	PMID     string `json:"pmid"`
	PMCID    string `json:"pmcid"`
}
