package biorxiv

// SearchResponse represents the top-level Europe PMC search API response.
type SearchResponse struct {
	HitCount       int        `json:"hitCount"`
	NextCursorMark string     `json:"nextCursorMark"`
	ResultList     ResultList `json:"resultList"`
}

// ResultList wraps the array of article results.
type ResultList struct {
	Result []Article `json:"result"`
}

// Article represents a single article in the Europe PMC response.
type Article struct {
	ID                   string `json:"id"`
	Source               string `json:"source"`               // "PPR"
	PMID                 string `json:"pmid"`
	PMCID                string `json:"pmcid"`
	DOI                  string `json:"doi"`
	Title                string `json:"title"`
	AuthorString         string `json:"authorString"`         // "Author A, Author B"
	JournalTitle         string `json:"journalTitle"`
	JournalVolume        string `json:"journalVolume"`
	PubYear              string `json:"pubYear"`
	AbstractText         string `json:"abstractText"`
	IsOpenAccess         string `json:"isOpenAccess"`         // "Y"/"N"
	CitedByCount         int    `json:"citedByCount"`
	FirstPublicationDate string `json:"firstPublicationDate"` // "2024-01-15"
	PublisherName        string `json:"publisherName"`        // "bioRxiv" or "medRxiv"
}
