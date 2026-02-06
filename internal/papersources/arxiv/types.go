package arxiv

import "encoding/xml"

// Feed represents the Atom XML response from the arXiv API.
type Feed struct {
	XMLName      xml.Name `xml:"feed"`
	TotalResults int      `xml:"totalResults"`
	StartIndex   int      `xml:"startIndex"`
	ItemsPerPage int      `xml:"itemsPerPage"`
	Entries      []Entry  `xml:"entry"`
}

// Entry represents a single arXiv paper in the Atom feed.
type Entry struct {
	ID              string     `xml:"id"`        // "http://arxiv.org/abs/2301.12345v1"
	Title           string     `xml:"title"`
	Summary         string     `xml:"summary"`   // abstract
	Published       string     `xml:"published"` // "2023-01-15T18:30:00Z"
	Updated         string     `xml:"updated"`
	Authors         []Author   `xml:"author"`
	Categories      []Category `xml:"category"`
	Links           []Link     `xml:"link"`
	DOI             string     `xml:"doi"`
	JournalRef      string     `xml:"journal_ref"`
	Comment         string     `xml:"comment"`
	PrimaryCategory Category   `xml:"primary_category"`
}

// Author represents a paper author in the arXiv Atom feed.
type Author struct {
	Name        string `xml:"name"`
	Affiliation string `xml:"affiliation"`
}

// Category represents an arXiv subject category.
type Category struct {
	Term string `xml:"term,attr"`
}

// Link represents a link element in the Atom feed.
type Link struct {
	Href  string `xml:"href,attr"`
	Rel   string `xml:"rel,attr"`
	Type  string `xml:"type,attr"`
	Title string `xml:"title,attr"`
}
