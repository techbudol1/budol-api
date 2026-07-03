package newsagent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type stubFetcher struct {
	mu       sync.Mutex
	articles []Article
	err      error
	calls    int
}

func (f *stubFetcher) Fetch(context.Context, string, int) ([]Article, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return append([]Article(nil), f.articles...), f.err
}

func TestFallbackFetcherUsesPublisherFeedsWhenGDELTFails(t *testing.T) {
	primary := &stubFetcher{err: errors.New("status 429")}
	fallback := &stubFetcher{articles: []Article{{Title: "Publisher fallback headline", URL: "https://example.com/story"}}}

	articles, err := NewFallbackFetcher(primary, fallback).Fetch(context.Background(), "Philippines", 40)
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if len(articles) != 1 || primary.calls != 1 || fallback.calls != 1 {
		t.Fatalf("unexpected fallback result: articles=%d primary=%d fallback=%d", len(articles), primary.calls, fallback.calls)
	}
}

func TestCachedFetcherAvoidsImmediateDuplicateRetrieval(t *testing.T) {
	source := &stubFetcher{articles: []Article{{Title: "Cached headline", URL: "https://example.com/story"}}}
	fetcher := NewCachedFetcher(source, time.Minute)

	for range 2 {
		if _, err := fetcher.Fetch(context.Background(), "Philippines", 40); err != nil {
			t.Fatalf("Fetch returned error: %v", err)
		}
	}
	if source.calls != 1 {
		t.Fatalf("source calls = %d, want 1", source.calls)
	}
}
