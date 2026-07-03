package newsagent

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestGDELTFetcherRetriesRateLimit(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"articles":[{"url":"https://inquirer.net/test","title":"A sufficiently long test headline","seendate":"20260627T120000Z","domain":"inquirer.net","sourcecountry":"Philippines"}]}`)
	}))
	defer server.Close()

	fetcher := NewGDELTFetcher(server.URL, map[string]bool{"inquirer.net": true}, nil)
	articles, err := fetcher.Fetch(t.Context(), "Philippines", 40)
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if len(articles) != 1 || calls.Load() != 3 {
		t.Fatalf("unexpected result: articles=%d calls=%d", len(articles), calls.Load())
	}
}
