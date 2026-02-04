// Package semanticscholar provides a client for the Semantic Scholar API.
//
// Semantic Scholar is a free, AI-powered research tool for scientific literature.
// This package implements the papersources.PaperSource interface for searching
// and retrieving academic papers from the Semantic Scholar Graph API.
//
// API Documentation: https://api.semanticscholar.org/api-docs/
package semanticscholar

// SearchResponse represents the response from the Semantic Scholar paper search endpoint.
type SearchResponse struct {
	// Total is the total number of papers matching the query.
	Total int `json:"total"`

	// Offset is the current offset in the result set.
	Offset int `json:"offset"`

	// Next is the offset for the next page of results.
	// A value of 0 indicates no more results.
	Next int `json:"next"`

	// Data contains the list of papers returned by the search.
	Data []PaperResult `json:"data"`
}

// PaperResult represents a single paper in the Semantic Scholar API response.
type PaperResult struct {
	// PaperID is the Semantic Scholar unique identifier for the paper.
	PaperID string `json:"paperId"`

	// Title is the title of the paper.
	Title string `json:"title"`

	// Abstract is the paper's abstract text.
	Abstract string `json:"abstract"`

	// Year is the publication year.
	Year int `json:"year"`

	// PublicationDate is the full publication date in YYYY-MM-DD format.
	PublicationDate string `json:"publicationDate"`

	// Venue is the publication venue (conference, journal name, etc.).
	Venue string `json:"venue"`

	// Journal contains journal-specific information if published in a journal.
	Journal *Journal `json:"journal,omitempty"`

	// Authors is the list of paper authors.
	Authors []Author `json:"authors"`

	// CitationCount is the number of citations this paper has received.
	CitationCount int `json:"citationCount"`

	// ReferenceCount is the number of references in this paper.
	ReferenceCount int `json:"referenceCount"`

	// IsOpenAccess indicates whether the paper is open access.
	IsOpenAccess bool `json:"isOpenAccess"`

	// OpenAccessPDF contains information about the open access PDF if available.
	OpenAccessPDF *OpenAccessPDF `json:"openAccessPdf,omitempty"`

	// ExternalIDs contains external identifiers for the paper (DOI, ArXiv, etc.).
	ExternalIDs *ExternalIDs `json:"externalIds,omitempty"`
}

// ExternalIDs contains external identifiers for a paper.
type ExternalIDs struct {
	// DOI is the Digital Object Identifier.
	DOI string `json:"DOI,omitempty"`

	// ArXiv is the ArXiv identifier.
	ArXiv string `json:"ArXiv,omitempty"`

	// PubMed is the PubMed identifier.
	PubMed string `json:"PubMed,omitempty"`

	// PubMedCentral is the PubMed Central identifier.
	PubMedCentral string `json:"PubMedCentral,omitempty"`
}

// Journal contains journal-specific information.
type Journal struct {
	// Name is the name of the journal.
	Name string `json:"name,omitempty"`

	// Volume is the journal volume.
	Volume string `json:"volume,omitempty"`

	// Pages is the page range (e.g., "1-15").
	Pages string `json:"pages,omitempty"`
}

// Author represents a paper author in the Semantic Scholar API.
type Author struct {
	// AuthorID is the Semantic Scholar unique identifier for the author.
	AuthorID string `json:"authorId,omitempty"`

	// Name is the author's name.
	Name string `json:"name"`
}

// OpenAccessPDF contains information about an open access PDF.
type OpenAccessPDF struct {
	// URL is the direct URL to the PDF.
	URL string `json:"url,omitempty"`

	// Status indicates the open access status (e.g., "HYBRID", "GOLD", "GREEN").
	Status string `json:"status,omitempty"`
}

// ErrorResponse represents an error response from the Semantic Scholar API.
type ErrorResponse struct {
	// Error is the error message from the API.
	Error string `json:"error,omitempty"`

	// Message is an alternative error message field.
	Message string `json:"message,omitempty"`
}
