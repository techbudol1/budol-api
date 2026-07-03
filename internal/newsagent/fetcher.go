package newsagent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type FallbackFetcher struct {
	primary  Fetcher
	fallback Fetcher
}

func NewFallbackFetcher(primary Fetcher, fallback Fetcher) *FallbackFetcher {
	return &FallbackFetcher{primary: primary, fallback: fallback}
}

func (f *FallbackFetcher) Fetch(ctx context.Context, query string, limit int) ([]Article, error) {
	articles, primaryErr := f.primary.Fetch(ctx, query, limit)
	if primaryErr == nil && len(articles) > 0 {
		return articles, nil
	}
	fallbackArticles, fallbackErr := f.fallback.Fetch(ctx, query, limit)
	if fallbackErr == nil {
		return fallbackArticles, nil
	}
	if primaryErr == nil {
		primaryErr = fmt.Errorf("no allowed articles returned")
	}
	return nil, fmt.Errorf("news retrieval failed (GDELT: %v; publisher RSS: %v)", primaryErr, fallbackErr)
}

type CachedFetcher struct {
	source Fetcher
	ttl    time.Duration

	mu        sync.Mutex
	query     string
	limit     int
	fetchedAt time.Time
	articles  []Article
}

func NewCachedFetcher(source Fetcher, ttl time.Duration) *CachedFetcher {
	return &CachedFetcher{source: source, ttl: ttl}
}

func (f *CachedFetcher) Fetch(ctx context.Context, query string, limit int) ([]Article, error) {
	f.mu.Lock()
	if query == f.query && limit == f.limit && len(f.articles) > 0 && time.Since(f.fetchedAt) < f.ttl {
		articles := append([]Article(nil), f.articles...)
		f.mu.Unlock()
		return articles, nil
	}
	f.mu.Unlock()

	articles, err := f.source.Fetch(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.query = query
	f.limit = limit
	f.fetchedAt = time.Now()
	f.articles = append([]Article(nil), articles...)
	f.mu.Unlock()
	return articles, nil
}
