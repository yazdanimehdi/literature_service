package pubmed

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/papersources"
)

// Sample XML responses for testing.
const esearchResponseXML = `<?xml version="1.0" encoding="UTF-8" ?>
<!DOCTYPE eSearchResult PUBLIC "-//NLM//DTD esearch 20060628//EN" "https://eutils.ncbi.nlm.nih.gov/eutils/dtd/20060628/esearch.dtd">
<eSearchResult>
	<Count>2</Count>
	<RetMax>2</RetMax>
	<RetStart>0</RetStart>
	<IdList>
		<Id>12345678</Id>
		<Id>87654321</Id>
	</IdList>
</eSearchResult>`

const esearchEmptyResponseXML = `<?xml version="1.0" encoding="UTF-8" ?>
<eSearchResult>
	<Count>0</Count>
	<RetMax>0</RetMax>
	<RetStart>0</RetStart>
	<IdList>
	</IdList>
</eSearchResult>`

const esearchPhraseNotFoundXML = `<?xml version="1.0" encoding="UTF-8" ?>
<eSearchResult>
	<Count>0</Count>
	<RetMax>0</RetMax>
	<RetStart>0</RetStart>
	<IdList>
	</IdList>
	<ErrorList>
		<PhraseNotFound>nonexistent_term_xyz</PhraseNotFound>
	</ErrorList>
</eSearchResult>`

const efetchResponseXML = `<?xml version="1.0" encoding="UTF-8" ?>
<!DOCTYPE PubmedArticleSet PUBLIC "-//NLM//DTD PubMedArticle, 1st January 2019//EN" "https://dtd.nlm.nih.gov/ncbi/pubmed/out/pubmed_190101.dtd">
<PubmedArticleSet>
	<PubmedArticle>
		<MedlineCitation Status="MEDLINE" Owner="NLM">
			<PMID Version="1">12345678</PMID>
			<Article PubModel="Print-Electronic">
				<Journal>
					<ISSN IssnType="Electronic">1234-5678</ISSN>
					<JournalIssue CitedMedium="Internet">
						<Volume>25</Volume>
						<Issue>3</Issue>
						<PubDate>
							<Year>2023</Year>
							<Month>Mar</Month>
							<Day>15</Day>
						</PubDate>
					</JournalIssue>
					<Title>Journal of Testing</Title>
					<ISOAbbreviation>J Test</ISOAbbreviation>
				</Journal>
				<ArticleTitle>CRISPR-Cas9 Gene Editing in Biomedical Research</ArticleTitle>
				<Pagination>
					<MedlinePgn>123-145</MedlinePgn>
				</Pagination>
				<ELocationID EIdType="doi" ValidYN="Y">10.1234/test.2023.001</ELocationID>
				<Abstract>
					<AbstractText Label="BACKGROUND" NlmCategory="BACKGROUND">Gene editing technologies have revolutionized biomedical research.</AbstractText>
					<AbstractText Label="METHODS" NlmCategory="METHODS">We analyzed CRISPR-Cas9 applications across multiple studies.</AbstractText>
					<AbstractText Label="RESULTS" NlmCategory="RESULTS">Our findings demonstrate significant improvements in editing efficiency.</AbstractText>
					<AbstractText Label="CONCLUSION" NlmCategory="CONCLUSIONS">CRISPR technology continues to advance therapeutic development.</AbstractText>
				</Abstract>
				<AuthorList CompleteYN="Y">
					<Author ValidYN="Y">
						<LastName>Smith</LastName>
						<ForeName>John A</ForeName>
						<Initials>JA</Initials>
						<AffiliationInfo>
							<Affiliation>Department of Genetics, University of Research</Affiliation>
						</AffiliationInfo>
						<Identifier Source="ORCID">0000-0001-2345-6789</Identifier>
					</Author>
					<Author ValidYN="Y">
						<LastName>Johnson</LastName>
						<ForeName>Emily</ForeName>
						<Initials>E</Initials>
						<AffiliationInfo>
							<Affiliation>Institute of Molecular Biology</Affiliation>
						</AffiliationInfo>
					</Author>
					<Author ValidYN="Y">
						<CollectiveName>CRISPR Research Consortium</CollectiveName>
					</Author>
				</AuthorList>
				<ArticleDate DateType="Electronic">
					<Year>2023</Year>
					<Month>02</Month>
					<Day>28</Day>
				</ArticleDate>
			</Article>
			<MeshHeadingList>
				<MeshHeading>
					<DescriptorName UI="D000090386" MajorTopicYN="N">CRISPR-Cas Systems</DescriptorName>
				</MeshHeading>
				<MeshHeading>
					<DescriptorName UI="D000077269" MajorTopicYN="N">Gene Editing</DescriptorName>
				</MeshHeading>
			</MeshHeadingList>
			<KeywordList Owner="NOTNLM">
				<Keyword MajorTopicYN="N">CRISPR</Keyword>
				<Keyword MajorTopicYN="N">Gene editing</Keyword>
				<Keyword MajorTopicYN="N">Therapeutics</Keyword>
			</KeywordList>
		</MedlineCitation>
		<PubmedData>
			<PublicationStatus>ppublish</PublicationStatus>
			<ArticleIdList>
				<ArticleId IdType="pubmed">12345678</ArticleId>
				<ArticleId IdType="doi">10.1234/test.2023.001</ArticleId>
				<ArticleId IdType="pmc">PMC9876543</ArticleId>
			</ArticleIdList>
		</PubmedData>
	</PubmedArticle>
	<PubmedArticle>
		<MedlineCitation Status="MEDLINE" Owner="NLM">
			<PMID Version="1">87654321</PMID>
			<Article PubModel="Print">
				<Journal>
					<JournalIssue CitedMedium="Print">
						<Volume>10</Volume>
						<PubDate>
							<MedlineDate>2022 Jan-Feb</MedlineDate>
						</PubDate>
					</JournalIssue>
					<Title>Molecular Therapy Methods</Title>
					<ISOAbbreviation>Mol Ther Methods</ISOAbbreviation>
				</Journal>
				<ArticleTitle>Advances in Gene Therapy Delivery Systems</ArticleTitle>
				<Pagination>
					<StartPage>50</StartPage>
					<EndPage>75</EndPage>
				</Pagination>
				<Abstract>
					<AbstractText>This review covers recent advances in viral and non-viral delivery systems for gene therapy applications.</AbstractText>
				</Abstract>
				<AuthorList CompleteYN="Y">
					<Author ValidYN="Y">
						<LastName>Brown</LastName>
						<ForeName>Michael</ForeName>
						<Initials>M</Initials>
					</Author>
				</AuthorList>
			</Article>
		</MedlineCitation>
		<PubmedData>
			<PublicationStatus>ppublish</PublicationStatus>
			<ArticleIdList>
				<ArticleId IdType="pubmed">87654321</ArticleId>
				<ArticleId IdType="doi">10.5678/mol.2022.050</ArticleId>
			</ArticleIdList>
		</PubmedData>
	</PubmedArticle>
</PubmedArticleSet>`

const efetchSingleArticleXML = `<?xml version="1.0" encoding="UTF-8" ?>
<PubmedArticleSet>
	<PubmedArticle>
		<MedlineCitation Status="MEDLINE" Owner="NLM">
			<PMID Version="1">12345678</PMID>
			<Article PubModel="Print">
				<Journal>
					<JournalIssue CitedMedium="Internet">
						<Volume>25</Volume>
						<Issue>3</Issue>
						<PubDate>
							<Year>2023</Year>
							<Month>Mar</Month>
						</PubDate>
					</JournalIssue>
					<Title>Journal of Testing</Title>
				</Journal>
				<ArticleTitle>Single Article Test</ArticleTitle>
				<Abstract>
					<AbstractText>Test abstract content.</AbstractText>
				</Abstract>
				<AuthorList CompleteYN="Y">
					<Author ValidYN="Y">
						<LastName>Test</LastName>
						<ForeName>Author</ForeName>
					</Author>
				</AuthorList>
			</Article>
		</MedlineCitation>
		<PubmedData>
			<PublicationStatus>ppublish</PublicationStatus>
			<ArticleIdList>
				<ArticleId IdType="pubmed">12345678</ArticleId>
			</ArticleIdList>
		</PubmedData>
	</PubmedArticle>
</PubmedArticleSet>`

const efetchEmptyResponseXML = `<?xml version="1.0" encoding="UTF-8" ?>
<PubmedArticleSet>
</PubmedArticleSet>`

func TestNewClient(t *testing.T) {
	t.Run("creates client with default config", func(t *testing.T) {
		cfg := Config{Enabled: true}
		client := New(cfg)

		require.NotNil(t, client)
		assert.Equal(t, DefaultBaseURL, client.config.BaseURL)
		assert.Equal(t, DefaultTimeout, client.config.Timeout)
		assert.Equal(t, DefaultRateLimit, client.config.RateLimit)
		assert.Equal(t, DefaultBurstSize, client.config.BurstSize)
		assert.Equal(t, DefaultMaxResults, client.config.MaxResults)
		assert.True(t, client.IsEnabled())
	})

	t.Run("creates client with custom config", func(t *testing.T) {
		cfg := Config{
			BaseURL:    "https://custom.api.example.com",
			APIKey:     "test-api-key",
			Timeout:    60 * time.Second,
			RateLimit:  10.0,
			BurstSize:  5,
			MaxResults: 50,
			Enabled:    true,
		}
		client := New(cfg)

		require.NotNil(t, client)
		assert.Equal(t, cfg.BaseURL, client.config.BaseURL)
		assert.Equal(t, cfg.APIKey, client.config.APIKey)
		assert.Equal(t, cfg.Timeout, client.config.Timeout)
		assert.Equal(t, cfg.RateLimit, client.config.RateLimit)
		assert.Equal(t, cfg.BurstSize, client.config.BurstSize)
		assert.Equal(t, cfg.MaxResults, client.config.MaxResults)
	})

	t.Run("creates disabled client", func(t *testing.T) {
		cfg := Config{Enabled: false}
		client := New(cfg)

		require.NotNil(t, client)
		assert.False(t, client.IsEnabled())
	})
}

func TestClient_SourceType(t *testing.T) {
	client := New(Config{Enabled: true})
	assert.Equal(t, domain.SourceTypePubMed, client.SourceType())
}

func TestClient_Name(t *testing.T) {
	client := New(Config{Enabled: true})
	assert.Equal(t, "PubMed", client.Name())
}

func TestClient_IsEnabled(t *testing.T) {
	t.Run("returns true when enabled", func(t *testing.T) {
		client := New(Config{Enabled: true})
		assert.True(t, client.IsEnabled())
	})

	t.Run("returns false when disabled", func(t *testing.T) {
		client := New(Config{Enabled: false})
		assert.False(t, client.IsEnabled())
	})
}

// Verify that Client implements PaperSource interface.
func TestClient_ImplementsPaperSource(t *testing.T) {
	var _ papersources.PaperSource = (*Client)(nil)
}

func TestClient_Search(t *testing.T) {
	t.Run("successful search with results", func(t *testing.T) {
		// Create test server that handles both esearch and efetch
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "esearch.fcgi") {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(esearchResponseXML))
			} else if strings.Contains(r.URL.Path, "efetch.fcgi") {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(efetchResponseXML))
			}
		}))
		defer server.Close()

		client := createTestClient(server.URL, true)

		params := papersources.SearchParams{
			Query:      "CRISPR gene editing",
			MaxResults: 10,
		}

		result, err := client.Search(context.Background(), params)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 2, result.TotalResults)
		assert.Len(t, result.Papers, 2)
		assert.Equal(t, domain.SourceTypePubMed, result.Source)
		assert.False(t, result.HasMore)

		// Verify first paper
		paper1 := result.Papers[0]
		assert.Equal(t, "CRISPR-Cas9 Gene Editing in Biomedical Research", paper1.Title)
		assert.Equal(t, "doi:10.1234/test.2023.001", paper1.CanonicalID)
		assert.Equal(t, "Journal of Testing", paper1.Journal)
		assert.Equal(t, "25", paper1.Volume)
		assert.Equal(t, "3", paper1.Issue)
		assert.Equal(t, "123-145", paper1.Pages)
		assert.Equal(t, 2023, paper1.PublicationYear)
		require.NotNil(t, paper1.PublicationDate)
		assert.Equal(t, 2023, paper1.PublicationDate.Year())

		// Verify authors
		require.Len(t, paper1.Authors, 3)
		assert.Equal(t, "John A Smith", paper1.Authors[0].Name)
		assert.Equal(t, "Department of Genetics, University of Research", paper1.Authors[0].Affiliation)
		assert.Equal(t, "0000-0001-2345-6789", paper1.Authors[0].ORCID)
		assert.Equal(t, "Emily Johnson", paper1.Authors[1].Name)
		assert.Equal(t, "CRISPR Research Consortium", paper1.Authors[2].Name)

		// Verify abstract (concatenated sections)
		assert.Contains(t, paper1.Abstract, "BACKGROUND:")
		assert.Contains(t, paper1.Abstract, "Gene editing technologies")
		assert.Contains(t, paper1.Abstract, "METHODS:")
		assert.Contains(t, paper1.Abstract, "RESULTS:")
		assert.Contains(t, paper1.Abstract, "CONCLUSION:")

		// Verify raw metadata
		assert.Equal(t, "12345678", paper1.RawMetadata["pmid"])
		assert.Equal(t, "10.1234/test.2023.001", paper1.RawMetadata["doi"])
		assert.Equal(t, "PMC9876543", paper1.RawMetadata["pmcid"])
		meshTerms := paper1.RawMetadata["mesh_terms"].([]string)
		assert.Contains(t, meshTerms, "CRISPR-Cas Systems")
		keywords := paper1.RawMetadata["keywords"].([]string)
		assert.Contains(t, keywords, "CRISPR")

		// Verify second paper
		paper2 := result.Papers[1]
		assert.Equal(t, "Advances in Gene Therapy Delivery Systems", paper2.Title)
		assert.Equal(t, "doi:10.5678/mol.2022.050", paper2.CanonicalID)
		assert.Equal(t, "50-75", paper2.Pages)
		assert.Equal(t, 2022, paper2.PublicationYear)
	})

	t.Run("search with date filters", func(t *testing.T) {
		var receivedQuery string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "esearch.fcgi") {
				receivedQuery = r.URL.RawQuery
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(esearchEmptyResponseXML))
			}
		}))
		defer server.Close()

		client := createTestClient(server.URL, true)

		fromDate := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
		toDate := time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC)

		params := papersources.SearchParams{
			Query:      "test",
			DateFrom:   &fromDate,
			DateTo:     &toDate,
			MaxResults: 10,
		}

		_, err := client.Search(context.Background(), params)
		require.NoError(t, err)

		assert.Contains(t, receivedQuery, "datetype=pdat")
		assert.Contains(t, receivedQuery, "mindate=2022%2F01%2F01")
		assert.Contains(t, receivedQuery, "maxdate=2023%2F12%2F31")
	})

	t.Run("search with pagination", func(t *testing.T) {
		var receivedOffset string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "esearch.fcgi") {
				receivedOffset = r.URL.Query().Get("retstart")
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(esearchResponseXML))
			} else if strings.Contains(r.URL.Path, "efetch.fcgi") {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(efetchResponseXML))
			}
		}))
		defer server.Close()

		client := createTestClient(server.URL, true)

		params := papersources.SearchParams{
			Query:      "test",
			Offset:     50,
			MaxResults: 10,
		}

		_, err := client.Search(context.Background(), params)
		require.NoError(t, err)

		assert.Equal(t, "50", receivedOffset)
	})

	t.Run("search with API key", func(t *testing.T) {
		var receivedAPIKey string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedAPIKey = r.URL.Query().Get("api_key")
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(esearchEmptyResponseXML))
		}))
		defer server.Close()

		httpClient := papersources.NewHTTPClient(papersources.HTTPClientConfig{
			RateLimit: 100,
			BurstSize: 10,
		})

		client := NewWithHTTPClient(Config{
			BaseURL: server.URL,
			APIKey:  "test-api-key-123",
			Enabled: true,
		}, httpClient)

		params := papersources.SearchParams{Query: "test"}
		_, err := client.Search(context.Background(), params)
		require.NoError(t, err)

		assert.Equal(t, "test-api-key-123", receivedAPIKey)
	})

	t.Run("search returns empty results", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(esearchEmptyResponseXML))
		}))
		defer server.Close()

		client := createTestClient(server.URL, true)

		params := papersources.SearchParams{Query: "nonexistent query xyz"}
		result, err := client.Search(context.Background(), params)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 0, result.TotalResults)
		assert.Empty(t, result.Papers)
		assert.False(t, result.HasMore)
	})

	t.Run("search handles phrase not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(esearchPhraseNotFoundXML))
		}))
		defer server.Close()

		client := createTestClient(server.URL, true)

		params := papersources.SearchParams{Query: "nonexistent_term_xyz"}
		result, err := client.Search(context.Background(), params)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 0, result.TotalResults)
		assert.Empty(t, result.Papers)
	})

	t.Run("search fails when disabled", func(t *testing.T) {
		client := New(Config{Enabled: false})

		params := papersources.SearchParams{Query: "test"}
		_, err := client.Search(context.Background(), params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "disabled")
	})

	t.Run("search handles esearch error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal Server Error"))
		}))
		defer server.Close()

		client := createTestClient(server.URL, true)

		params := papersources.SearchParams{Query: "test"}
		_, err := client.Search(context.Background(), params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "esearch failed")
	})

	t.Run("search handles efetch error", func(t *testing.T) {
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if strings.Contains(r.URL.Path, "esearch.fcgi") {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(esearchResponseXML))
			} else if strings.Contains(r.URL.Path, "efetch.fcgi") {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Internal Server Error"))
			}
		}))
		defer server.Close()

		client := createTestClient(server.URL, true)

		params := papersources.SearchParams{Query: "test"}
		_, err := client.Search(context.Background(), params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "efetch failed")
	})

	t.Run("search respects context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := createTestClient(server.URL, true)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		params := papersources.SearchParams{Query: "test"}
		_, err := client.Search(ctx, params)
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "canceled"))
	})
}

func TestClient_GetByID(t *testing.T) {
	t.Run("successful get by ID", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "efetch.fcgi") {
				// Verify the ID parameter
				ids := r.URL.Query().Get("id")
				assert.Equal(t, "12345678", ids)

				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(efetchSingleArticleXML))
			}
		}))
		defer server.Close()

		client := createTestClient(server.URL, true)

		paper, err := client.GetByID(context.Background(), "12345678")
		require.NoError(t, err)
		require.NotNil(t, paper)

		assert.Equal(t, "Single Article Test", paper.Title)
		assert.Equal(t, "pubmed:12345678", paper.CanonicalID)
		assert.Equal(t, "Journal of Testing", paper.Journal)
		assert.Equal(t, 2023, paper.PublicationYear)
		assert.Len(t, paper.Authors, 1)
		assert.Equal(t, "Author Test", paper.Authors[0].Name)
	})

	t.Run("get by ID not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(efetchEmptyResponseXML))
		}))
		defer server.Close()

		client := createTestClient(server.URL, true)

		_, err := client.GetByID(context.Background(), "99999999")
		require.Error(t, err)
		assert.True(t, errors.Is(err, domain.ErrNotFound))
	})

	t.Run("get by ID fails when disabled", func(t *testing.T) {
		client := New(Config{Enabled: false})

		_, err := client.GetByID(context.Background(), "12345678")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "disabled")
	})

	t.Run("get by ID handles server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		client := createTestClient(server.URL, true)

		_, err := client.GetByID(context.Background(), "12345678")
		require.Error(t, err)
	})
}

func TestClient_articleToPaper(t *testing.T) {
	client := New(Config{Enabled: true})

	t.Run("extracts DOI from ELocationID", func(t *testing.T) {
		article := PubmedArticle{
			MedlineCitation: MedlineCitation{
				PMID: PMID{Value: "12345"},
				Article: Article{
					ArticleTitle: "Test Article",
					ELocationID: []ELocationID{
						{EIdType: "doi", Valid: "Y", Value: "10.1234/test"},
					},
					Journal: Journal{
						JournalIssue: JournalIssue{
							PubDate: PubDate{Year: "2023"},
						},
					},
				},
			},
			PubmedData: PubmedData{
				ArticleIdList: ArticleIdList{},
			},
		}

		paper := client.articleToPaper(article)
		assert.Equal(t, "doi:10.1234/test", paper.CanonicalID)
	})

	t.Run("extracts DOI from ArticleIdList when ELocationID missing", func(t *testing.T) {
		article := PubmedArticle{
			MedlineCitation: MedlineCitation{
				PMID: PMID{Value: "12345"},
				Article: Article{
					ArticleTitle: "Test Article",
					Journal: Journal{
						JournalIssue: JournalIssue{
							PubDate: PubDate{Year: "2023"},
						},
					},
				},
			},
			PubmedData: PubmedData{
				ArticleIdList: ArticleIdList{
					ArticleIds: []ArticleId{
						{IdType: "pubmed", Value: "12345"},
						{IdType: "doi", Value: "10.5678/article"},
					},
				},
			},
		}

		paper := client.articleToPaper(article)
		assert.Equal(t, "doi:10.5678/article", paper.CanonicalID)
	})

	t.Run("uses PMID when no DOI available", func(t *testing.T) {
		article := PubmedArticle{
			MedlineCitation: MedlineCitation{
				PMID: PMID{Value: "12345"},
				Article: Article{
					ArticleTitle: "Test Article",
					Journal: Journal{
						JournalIssue: JournalIssue{
							PubDate: PubDate{Year: "2023"},
						},
					},
				},
			},
			PubmedData: PubmedData{
				ArticleIdList: ArticleIdList{
					ArticleIds: []ArticleId{
						{IdType: "pubmed", Value: "12345"},
					},
				},
			},
		}

		paper := client.articleToPaper(article)
		assert.Equal(t, "pubmed:12345", paper.CanonicalID)
	})

	t.Run("skips invalid DOI", func(t *testing.T) {
		article := PubmedArticle{
			MedlineCitation: MedlineCitation{
				PMID: PMID{Value: "12345"},
				Article: Article{
					ArticleTitle: "Test Article",
					ELocationID: []ELocationID{
						{EIdType: "doi", Valid: "N", Value: "invalid-doi"},
					},
					Journal: Journal{
						JournalIssue: JournalIssue{
							PubDate: PubDate{Year: "2023"},
						},
					},
				},
			},
			PubmedData: PubmedData{
				ArticleIdList: ArticleIdList{},
			},
		}

		paper := client.articleToPaper(article)
		assert.Equal(t, "pubmed:12345", paper.CanonicalID)
	})

	t.Run("handles MedlineDate format", func(t *testing.T) {
		article := PubmedArticle{
			MedlineCitation: MedlineCitation{
				PMID: PMID{Value: "12345"},
				Article: Article{
					ArticleTitle: "Test Article",
					Journal: Journal{
						JournalIssue: JournalIssue{
							PubDate: PubDate{
								MedlineDate: "2022 Jan-Feb",
							},
						},
					},
				},
			},
		}

		paper := client.articleToPaper(article)
		assert.Equal(t, 2022, paper.PublicationYear)
		require.NotNil(t, paper.PublicationDate)
		assert.Equal(t, 2022, paper.PublicationDate.Year())
	})

	t.Run("uses electronic publication date when available", func(t *testing.T) {
		article := PubmedArticle{
			MedlineCitation: MedlineCitation{
				PMID: PMID{Value: "12345"},
				Article: Article{
					ArticleTitle: "Test Article",
					Journal: Journal{
						JournalIssue: JournalIssue{
							PubDate: PubDate{Year: "2023", Month: "Dec"},
						},
					},
					ArticleDate: []ArticleDate{
						{DateType: "Electronic", Year: "2023", Month: "06", Day: "15"},
					},
				},
			},
		}

		paper := client.articleToPaper(article)
		require.NotNil(t, paper.PublicationDate)
		assert.Equal(t, time.June, paper.PublicationDate.Month())
		assert.Equal(t, 15, paper.PublicationDate.Day())
	})

	t.Run("concatenates structured abstract sections", func(t *testing.T) {
		article := PubmedArticle{
			MedlineCitation: MedlineCitation{
				PMID: PMID{Value: "12345"},
				Article: Article{
					ArticleTitle: "Test Article",
					Abstract: &Abstract{
						AbstractTexts: []AbstractText{
							{Label: "BACKGROUND", Value: "Background text."},
							{Label: "METHODS", Value: "Methods text."},
							{Label: "RESULTS", Value: "Results text."},
						},
					},
					Journal: Journal{
						JournalIssue: JournalIssue{
							PubDate: PubDate{Year: "2023"},
						},
					},
				},
			},
		}

		paper := client.articleToPaper(article)
		assert.Contains(t, paper.Abstract, "BACKGROUND: Background text.")
		assert.Contains(t, paper.Abstract, "METHODS: Methods text.")
		assert.Contains(t, paper.Abstract, "RESULTS: Results text.")
	})

	t.Run("handles single unlabeled abstract", func(t *testing.T) {
		article := PubmedArticle{
			MedlineCitation: MedlineCitation{
				PMID: PMID{Value: "12345"},
				Article: Article{
					ArticleTitle: "Test Article",
					Abstract: &Abstract{
						AbstractTexts: []AbstractText{
							{Value: "Simple abstract without sections."},
						},
					},
					Journal: Journal{
						JournalIssue: JournalIssue{
							PubDate: PubDate{Year: "2023"},
						},
					},
				},
			},
		}

		paper := client.articleToPaper(article)
		assert.Equal(t, "Simple abstract without sections.", paper.Abstract)
	})

	t.Run("skips invalid authors", func(t *testing.T) {
		article := PubmedArticle{
			MedlineCitation: MedlineCitation{
				PMID: PMID{Value: "12345"},
				Article: Article{
					ArticleTitle: "Test Article",
					AuthorList: &AuthorList{
						Authors: []Author{
							{ValidYN: "Y", LastName: "Valid", ForeName: "Author"},
							{ValidYN: "N", LastName: "Invalid", ForeName: "Author"},
							{ValidYN: "Y", LastName: "Another", ForeName: "Valid"},
						},
					},
					Journal: Journal{
						JournalIssue: JournalIssue{
							PubDate: PubDate{Year: "2023"},
						},
					},
				},
			},
		}

		paper := client.articleToPaper(article)
		require.Len(t, paper.Authors, 2)
		assert.Equal(t, "Author Valid", paper.Authors[0].Name)
		assert.Equal(t, "Valid Another", paper.Authors[1].Name)
	})

	t.Run("handles collective name author", func(t *testing.T) {
		article := PubmedArticle{
			MedlineCitation: MedlineCitation{
				PMID: PMID{Value: "12345"},
				Article: Article{
					ArticleTitle: "Test Article",
					AuthorList: &AuthorList{
						Authors: []Author{
							{CollectiveName: "Research Consortium"},
						},
					},
					Journal: Journal{
						JournalIssue: JournalIssue{
							PubDate: PubDate{Year: "2023"},
						},
					},
				},
			},
		}

		paper := client.articleToPaper(article)
		require.Len(t, paper.Authors, 1)
		assert.Equal(t, "Research Consortium", paper.Authors[0].Name)
	})

	t.Run("extracts pages from MedlinePgn", func(t *testing.T) {
		article := PubmedArticle{
			MedlineCitation: MedlineCitation{
				PMID: PMID{Value: "12345"},
				Article: Article{
					ArticleTitle: "Test Article",
					Pagination: &Pagination{
						MedlinePgn: "123-145",
					},
					Journal: Journal{
						JournalIssue: JournalIssue{
							PubDate: PubDate{Year: "2023"},
						},
					},
				},
			},
		}

		paper := client.articleToPaper(article)
		assert.Equal(t, "123-145", paper.Pages)
	})

	t.Run("extracts pages from StartPage and EndPage", func(t *testing.T) {
		article := PubmedArticle{
			MedlineCitation: MedlineCitation{
				PMID: PMID{Value: "12345"},
				Article: Article{
					ArticleTitle: "Test Article",
					Pagination: &Pagination{
						StartPage: "50",
						EndPage:   "75",
					},
					Journal: Journal{
						JournalIssue: JournalIssue{
							PubDate: PubDate{Year: "2023"},
						},
					},
				},
			},
		}

		paper := client.articleToPaper(article)
		assert.Equal(t, "50-75", paper.Pages)
	})

	t.Run("uses ISOAbbreviation when Title is empty", func(t *testing.T) {
		article := PubmedArticle{
			MedlineCitation: MedlineCitation{
				PMID: PMID{Value: "12345"},
				Article: Article{
					ArticleTitle: "Test Article",
					Journal: Journal{
						ISOAbbreviation: "J Abbrev",
						JournalIssue: JournalIssue{
							PubDate: PubDate{Year: "2023"},
						},
					},
				},
			},
		}

		paper := client.articleToPaper(article)
		assert.Equal(t, "J Abbrev", paper.Journal)
	})
}

func TestClient_parseMonth(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Month
	}{
		{"", time.January},
		{"1", time.January},
		{"01", time.January},
		{"6", time.June},
		{"12", time.December},
		{"Jan", time.January},
		{"jan", time.January},
		{"JANUARY", time.January},
		{"Feb", time.February},
		{"Mar", time.March},
		{"Apr", time.April},
		{"May", time.May},
		{"Jun", time.June},
		{"Jul", time.July},
		{"Aug", time.August},
		{"Sep", time.September},
		{"Oct", time.October},
		{"Nov", time.November},
		{"Dec", time.December},
		{"invalid", time.January},
		{"13", time.January}, // Out of range
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseMonth(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClient_extractYearFromMedlineDate(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"2020 Jan-Feb", 2020},
		{"2021 Spring", 2021},
		{"2019-2020", 2019},
		{"2022", 2022},
		{"Jan 2020", 0}, // Year not first
		{"", 0},
		{"invalid", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractYearFromMedlineDate(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// createTestClient creates a test client with the given base URL.
func createTestClient(baseURL string, enabled bool) *Client {
	httpClient := papersources.NewHTTPClient(papersources.HTTPClientConfig{
		RateLimit:  100,
		BurstSize:  10,
		MaxRetries: 0, // No retries in tests
	})

	return NewWithHTTPClient(Config{
		BaseURL: baseURL,
		Enabled: enabled,
	}, httpClient)
}
