package newsagent

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type RSSFetcher struct {
	feedURLs   []string
	client     *http.Client
	primarySet map[string]bool
	sourceSet  map[string]bool
}

type rssDocument struct {
	Channel struct {
		Items []struct {
			Title   string `xml:"title"`
			Link    string `xml:"link"`
			PubDate string `xml:"pubDate"`
		} `xml:"item"`
	} `xml:"channel"`
}

func NewRSSFetcher(feedURLs []string, sourceSet map[string]bool, primarySet map[string]bool) *RSSFetcher {
	return &RSSFetcher{
		feedURLs:   append([]string(nil), feedURLs...),
		client:     &http.Client{Timeout: 15 * time.Second},
		primarySet: primarySet,
		sourceSet:  sourceSet,
	}
}

func (f *RSSFetcher) Fetch(ctx context.Context, _ string, limit int) ([]Article, error) {
	if f == nil || len(f.feedURLs) == 0 {
		return nil, errors.New("publisher RSS feeds are not configured")
	}
	if limit <= 0 || limit > 250 {
		limit = 40
	}

	articles := make([]Article, 0, limit)
	seenURLs := make(map[string]bool)
	var feedErrors []string
	for _, feedURL := range f.feedURLs {
		feedArticles, err := f.fetchFeed(ctx, feedURL)
		if err != nil {
			feedErrors = append(feedErrors, err.Error())
			continue
		}
		for _, article := range feedArticles {
			if seenURLs[article.URL] {
				continue
			}
			seenURLs[article.URL] = true
			articles = append(articles, article)
		}
	}
	if len(articles) == 0 && len(feedErrors) > 0 {
		return nil, errors.New(strings.Join(feedErrors, "; "))
	}
	sortArticlesNewestFirst(articles)
	if len(articles) > limit {
		articles = articles[:limit]
	}
	return articles, nil
}

func (f *RSSFetcher) fetchFeed(ctx context.Context, feedURL string) ([]Article, error) {
	parsedFeedURL, err := url.Parse(strings.TrimSpace(feedURL))
	if err != nil || parsedFeedURL.Scheme != "https" || parsedFeedURL.Host == "" {
		return nil, fmt.Errorf("invalid publisher RSS URL %q", feedURL)
	}
	domain := normalizeDomain(parsedFeedURL.Hostname())
	if len(f.sourceSet) > 0 && !domainAllowed(domain, f.sourceSet) {
		return nil, fmt.Errorf("publisher RSS domain %q is not allowed", domain)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedFeedURL.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/rss+xml, application/xml, text/xml")
	request.Header.Set("User-Agent", "BudolPH-News-Scout/1.0")
	response, err := f.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", domain, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s RSS request failed: status %d", domain, response.StatusCode)
	}
	var document rssDocument
	if err := xml.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&document); err != nil {
		return nil, fmt.Errorf("%s returned invalid RSS: %w", domain, err)
	}

	articles := make([]Article, 0, len(document.Channel.Items))
	for _, item := range document.Channel.Items {
		itemURL, err := url.Parse(strings.TrimSpace(item.Link))
		if err != nil || itemURL.Scheme != "https" || !domainAllowed(normalizeDomain(itemURL.Hostname()), map[string]bool{domain: true}) {
			continue
		}
		title := strings.TrimSpace(item.Title)
		if len(title) < 15 {
			continue
		}
		articles = append(articles, Article{
			Title:       title,
			URL:         itemURL.String(),
			Domain:      domain,
			PublishedAt: normalizeRSSTime(item.PubDate),
			IsPrimary:   domainAllowed(domain, f.primarySet),
		})
	}
	return articles, nil
}

func normalizeRSSTime(value string) string {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
	}
	return value
}

func sortArticlesNewestFirst(articles []Article) {
	for i := 1; i < len(articles); i++ {
		for j := i; j > 0 && articles[j].PublishedAt > articles[j-1].PublishedAt; j-- {
			articles[j], articles[j-1] = articles[j-1], articles[j]
		}
	}
}
