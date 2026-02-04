// Package pubmed provides a client for the NCBI PubMed E-utilities API.
//
// PubMed is a biomedical literature database maintained by NCBI.
// This package implements the papersources.PaperSource interface
// to search and retrieve academic papers from PubMed.
//
// The E-utilities API documentation is available at:
// https://www.ncbi.nlm.nih.gov/books/NBK25499/
package pubmed

import "encoding/xml"

// ESearchResult represents the response from the esearch.fcgi endpoint.
// This endpoint returns a list of PMIDs matching a search query.
type ESearchResult struct {
	XMLName   xml.Name   `xml:"eSearchResult"`
	Count     int        `xml:"Count"`
	RetMax    int        `xml:"RetMax"`
	RetStart  int        `xml:"RetStart"`
	IDList    IDList     `xml:"IdList"`
	QueryKey  string     `xml:"QueryKey,omitempty"`
	WebEnv    string     `xml:"WebEnv,omitempty"`
	ErrorList *ErrorList `xml:"ErrorList,omitempty"`
}

// IDList contains the list of PMIDs returned by a search.
type IDList struct {
	IDs []string `xml:"Id"`
}

// ErrorList contains errors from the E-utilities API.
type ErrorList struct {
	PhraseNotFound []string `xml:"PhraseNotFound,omitempty"`
	FieldNotFound  []string `xml:"FieldNotFound,omitempty"`
}

// PubmedArticleSet represents the response from the efetch.fcgi endpoint.
// This endpoint returns full article metadata for a list of PMIDs.
type PubmedArticleSet struct {
	XMLName  xml.Name        `xml:"PubmedArticleSet"`
	Articles []PubmedArticle `xml:"PubmedArticle"`
}

// PubmedArticle represents a single article in the PubMed database.
type PubmedArticle struct {
	MedlineCitation MedlineCitation `xml:"MedlineCitation"`
	PubmedData      PubmedData      `xml:"PubmedData"`
}

// MedlineCitation contains the core bibliographic information.
type MedlineCitation struct {
	PMID            PMID             `xml:"PMID"`
	DateCompleted   *PubMedDate      `xml:"DateCompleted,omitempty"`
	DateRevised     *PubMedDate      `xml:"DateRevised,omitempty"`
	Article         Article          `xml:"Article"`
	MeshHeadingList *MeshHeadingList `xml:"MeshHeadingList,omitempty"`
	KeywordList     *KeywordList     `xml:"KeywordList,omitempty"`
}

// PMID represents the PubMed identifier with optional version.
type PMID struct {
	Version int    `xml:"Version,attr,omitempty"`
	Value   string `xml:",chardata"`
}

// PubMedDate represents a date in PubMed format.
type PubMedDate struct {
	Year  string `xml:"Year"`
	Month string `xml:"Month,omitempty"`
	Day   string `xml:"Day,omitempty"`
}

// Article contains the article metadata.
type Article struct {
	Journal             Journal              `xml:"Journal"`
	ArticleTitle        string               `xml:"ArticleTitle"`
	Pagination          *Pagination          `xml:"Pagination,omitempty"`
	ELocationID         []ELocationID        `xml:"ELocationID,omitempty"`
	Abstract            *Abstract            `xml:"Abstract,omitempty"`
	AuthorList          *AuthorList          `xml:"AuthorList,omitempty"`
	Language            []string             `xml:"Language,omitempty"`
	PublicationTypeList *PublicationTypeList `xml:"PublicationTypeList,omitempty"`
	ArticleDate         []ArticleDate        `xml:"ArticleDate,omitempty"`
}

// Journal contains journal information.
type Journal struct {
	ISSN            *ISSN        `xml:"ISSN,omitempty"`
	JournalIssue    JournalIssue `xml:"JournalIssue"`
	Title           string       `xml:"Title,omitempty"`
	ISOAbbreviation string       `xml:"ISOAbbreviation,omitempty"`
}

// ISSN represents the journal ISSN.
type ISSN struct {
	IssnType string `xml:"IssnType,attr,omitempty"`
	Value    string `xml:",chardata"`
}

// JournalIssue contains the volume, issue, and publication date.
type JournalIssue struct {
	CitedMedium string  `xml:"CitedMedium,attr,omitempty"`
	Volume      string  `xml:"Volume,omitempty"`
	Issue       string  `xml:"Issue,omitempty"`
	PubDate     PubDate `xml:"PubDate"`
}

// PubDate represents the publication date which may have various formats.
type PubDate struct {
	Year        string `xml:"Year,omitempty"`
	Month       string `xml:"Month,omitempty"`
	Day         string `xml:"Day,omitempty"`
	Season      string `xml:"Season,omitempty"`
	MedlineDate string `xml:"MedlineDate,omitempty"`
}

// Pagination contains page information.
type Pagination struct {
	StartPage  string `xml:"StartPage,omitempty"`
	EndPage    string `xml:"EndPage,omitempty"`
	MedlinePgn string `xml:"MedlinePgn,omitempty"`
}

// ELocationID represents an electronic location identifier (DOI or PII).
type ELocationID struct {
	EIdType string `xml:"EIdType,attr"`
	Valid   string `xml:"ValidYN,attr,omitempty"`
	Value   string `xml:",chardata"`
}

// Abstract contains the article abstract, which may have multiple sections.
type Abstract struct {
	AbstractTexts []AbstractText `xml:"AbstractText"`
	CopyrightInfo string         `xml:"CopyrightInformation,omitempty"`
}

// AbstractText represents a section of the abstract.
// Structured abstracts have labeled sections (Background, Methods, Results, etc.).
type AbstractText struct {
	Label       string `xml:"Label,attr,omitempty"`
	NlmCategory string `xml:"NlmCategory,attr,omitempty"`
	Value       string `xml:",chardata"`
}

// AuthorList contains the list of authors.
type AuthorList struct {
	CompleteYN string   `xml:"CompleteYN,attr,omitempty"`
	Authors    []Author `xml:"Author"`
}

// Author represents a single author with name and optional identifiers.
type Author struct {
	ValidYN         string            `xml:"ValidYN,attr,omitempty"`
	LastName        string            `xml:"LastName,omitempty"`
	ForeName        string            `xml:"ForeName,omitempty"`
	Initials        string            `xml:"Initials,omitempty"`
	Suffix          string            `xml:"Suffix,omitempty"`
	CollectiveName  string            `xml:"CollectiveName,omitempty"`
	Identifiers     []Identifier      `xml:"Identifier,omitempty"`
	AffiliationInfo []AffiliationInfo `xml:"AffiliationInfo,omitempty"`
}

// Identifier represents an author identifier (e.g., ORCID).
type Identifier struct {
	Source string `xml:"Source,attr"`
	Value  string `xml:",chardata"`
}

// AffiliationInfo contains author affiliation information.
type AffiliationInfo struct {
	Affiliation string       `xml:"Affiliation"`
	Identifiers []Identifier `xml:"Identifier,omitempty"`
}

// ArticleDate represents the article publication date.
type ArticleDate struct {
	DateType string `xml:"DateType,attr,omitempty"`
	Year     string `xml:"Year"`
	Month    string `xml:"Month,omitempty"`
	Day      string `xml:"Day,omitempty"`
}

// PublicationTypeList contains the publication types.
type PublicationTypeList struct {
	PublicationTypes []PublicationType `xml:"PublicationType"`
}

// PublicationType represents a publication type (e.g., Journal Article, Review).
type PublicationType struct {
	UI    string `xml:"UI,attr,omitempty"`
	Value string `xml:",chardata"`
}

// MeshHeadingList contains the MeSH terms assigned to the article.
type MeshHeadingList struct {
	MeshHeadings []MeshHeading `xml:"MeshHeading"`
}

// MeshHeading represents a MeSH descriptor with optional qualifiers.
type MeshHeading struct {
	DescriptorName DescriptorName  `xml:"DescriptorName"`
	QualifierNames []QualifierName `xml:"QualifierName,omitempty"`
}

// DescriptorName represents a MeSH descriptor.
type DescriptorName struct {
	UI         string `xml:"UI,attr,omitempty"`
	MajorTopic string `xml:"MajorTopicYN,attr,omitempty"`
	Value      string `xml:",chardata"`
}

// QualifierName represents a MeSH qualifier.
type QualifierName struct {
	UI         string `xml:"UI,attr,omitempty"`
	MajorTopic string `xml:"MajorTopicYN,attr,omitempty"`
	Value      string `xml:",chardata"`
}

// KeywordList contains author-provided keywords.
type KeywordList struct {
	Owner    string    `xml:"Owner,attr,omitempty"`
	Keywords []Keyword `xml:"Keyword"`
}

// Keyword represents a single keyword.
type Keyword struct {
	MajorTopic string `xml:"MajorTopicYN,attr,omitempty"`
	Value      string `xml:",chardata"`
}

// PubmedData contains additional PubMed-specific data.
type PubmedData struct {
	History           *History       `xml:"History,omitempty"`
	PublicationStatus string         `xml:"PublicationStatus,omitempty"`
	ArticleIdList     ArticleIdList  `xml:"ArticleIdList"`
	ReferenceList     *ReferenceList `xml:"ReferenceList,omitempty"`
}

// History contains the publication history dates.
type History struct {
	PubMedPubDates []PubMedPubDate `xml:"PubMedPubDate"`
}

// PubMedPubDate represents a date in the publication history.
type PubMedPubDate struct {
	PubStatus string `xml:"PubStatus,attr"`
	Year      string `xml:"Year"`
	Month     string `xml:"Month"`
	Day       string `xml:"Day"`
	Hour      string `xml:"Hour,omitempty"`
	Minute    string `xml:"Minute,omitempty"`
}

// ArticleIdList contains various identifiers for the article.
type ArticleIdList struct {
	ArticleIds []ArticleId `xml:"ArticleId"`
}

// ArticleId represents an article identifier (PMID, DOI, PMC, etc.).
type ArticleId struct {
	IdType string `xml:"IdType,attr"`
	Value  string `xml:",chardata"`
}

// ReferenceList contains references cited by the article.
type ReferenceList struct {
	References []Reference `xml:"Reference"`
}

// Reference represents a single reference.
type Reference struct {
	Citation      string         `xml:"Citation,omitempty"`
	ArticleIdList *ArticleIdList `xml:"ArticleIdList,omitempty"`
}
