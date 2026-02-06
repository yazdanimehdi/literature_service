package scopus

// SearchResponse represents the top-level Scopus search API response.
type SearchResponse struct {
	SearchResults SearchResults `json:"search-results"`
}

// SearchResults contains the search result metadata and entries.
type SearchResults struct {
	TotalResults string  `json:"opensearch:totalResults"`
	StartIndex   string  `json:"opensearch:startIndex"`
	ItemsPerPage string  `json:"opensearch:itemsPerPage"`
	Entries      []Entry `json:"entry"`
}

// Entry represents a single document in the Scopus search results.
type Entry struct {
	Identifier      string        `json:"dc:identifier"`          // "SCOPUS_ID:85012345678"
	EID             string        `json:"eid"`                    // "2-s2.0-85012345678"
	DOI             string        `json:"prism:doi"`
	Title           string        `json:"dc:title"`
	Creator         string        `json:"dc:creator"`             // first author only in STANDARD view
	Description     string        `json:"dc:description"`         // abstract (COMPLETE view)
	PublicationName string        `json:"prism:publicationName"`
	Volume          string        `json:"prism:volume"`
	IssueID         string        `json:"prism:issueIdentifier"`
	PageRange       string        `json:"prism:pageRange"`
	CoverDate       string        `json:"prism:coverDate"`        // "2024-01-15"
	CitedByCount    string        `json:"citedby-count"`
	PubMedID        string        `json:"pubmed-id"`
	Aggregation     string        `json:"prism:aggregationType"`
	SubType         string        `json:"subtype"`
	OpenAccess      string        `json:"openaccess"`             // "0" or "1"
	OpenAccessFlag  bool          `json:"openaccessFlag"`
	Affiliations    []Affiliation `json:"affiliation"`
	Authors         *AuthorGroup  `json:"author"`                 // COMPLETE view only
}

// Affiliation represents an author's institutional affiliation.
type Affiliation struct {
	Name    string `json:"affilname"`
	City    string `json:"affiliation-city"`
	Country string `json:"affiliation-country"`
}

// AuthorGroup wraps the author array in Scopus COMPLETE view responses.
type AuthorGroup struct {
	Authors []ScopusAuthor `json:"author"`
}

// ScopusAuthor represents a single author in the Scopus response.
type ScopusAuthor struct {
	AuthID    string `json:"authid"`
	Name      string `json:"authname"`    // "Surname, Given"
	GivenName string `json:"given-name"`
	Surname   string `json:"surname"`
	ORCID     string `json:"orcid"`
}
