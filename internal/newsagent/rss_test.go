package newsagent

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestRSSFetcherParsesAllowedPublisherFeed(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprintf(w, `<?xml version="1.0"?><rss><channel><item><title>A sufficiently long publisher headline</title><link>%s/story</link><pubDate>Sat, 27 Jun 2026 12:00:00 GMT</pubDate></item></channel></rss>`, server.URL)
	}))
	defer server.Close()

	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	domain := normalizeDomain(parsedURL.Hostname())
	fetcher := NewRSSFetcher([]string{server.URL}, map[string]bool{domain: true}, nil)
	fetcher.client = server.Client()
	articles, err := fetcher.Fetch(t.Context(), "", 40)
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if len(articles) != 1 {
		t.Fatalf("articles = %d, want 1", len(articles))
	}
}
