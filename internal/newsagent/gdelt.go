package newsagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type GDELTFetcher struct {
	baseURL    string
	client     *http.Client
	primarySet map[string]bool
	sourceSet  map[string]bool
}

const gdeltMaxAttempts = 3

func NewGDELTFetcher(baseURL string, sourceSet map[string]bool, primarySet map[string]bool) *GDELTFetcher {
	return &GDELTFetcher{
		baseURL:    strings.TrimSpace(baseURL),
		client:     &http.Client{Timeout: 25 * time.Second},
		primarySet: primarySet,
		sourceSet:  sourceSet,
	}
}

func (f *GDELTFetcher) Fetch(ctx context.Context, query string, limit int) ([]Article, error) {
	if f == nil || f.baseURL == "" {
		return nil, errors.New("GDELT source URL is not configured")
	}
	if limit <= 0 || limit > 250 {
		limit = 40
	}
	endpoint, err := url.Parse(f.baseURL)
	if err != nil {
		return nil, err
	}
	values := endpoint.Query()
	values.Set("query", strings.TrimSpace(query))
	values.Set("mode", "artlist")
	values.Set("maxrecords", strconv.Itoa(limit))
	values.Set("format", "json")
	values.Set("sort", "datedesc")
	values.Set("timespan", "24h")
	endpoint.RawQuery = values.Encode()

	var body []byte
	for attempt := 0; attempt < gdeltMaxAttempts; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("User-Agent", "BudolPH-News-Scout/1.0")
		response, err := f.client.Do(request)
		if err != nil {
			return nil, err
		}
		body, err = io.ReadAll(io.LimitReader(response.Body, 4<<20))
		response.Body.Close()
		if err != nil {
			return nil, err
		}
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			break
		}
		retryable := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		if !retryable || attempt == gdeltMaxAttempts-1 {
			return nil, fmt.Errorf("GDELT request failed after %d attempt(s): status %d", attempt+1, response.StatusCode)
		}
		delay := retryDelay(response.Header.Get("Retry-After"), attempt)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	var payload struct {
		Articles []struct {
			URL           string `json:"url"`
			Title         string `json:"title"`
			SeenDate      string `json:"seendate"`
			Domain        string `json:"domain"`
			SourceCountry string `json:"sourcecountry"`
		} `json:"articles"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid GDELT response: %w", err)
	}

	articles := make([]Article, 0, len(payload.Articles))
	seenURLs := map[string]bool{}
	for _, item := range payload.Articles {
		itemURL, err := url.Parse(strings.TrimSpace(item.URL))
		if err != nil || itemURL.Scheme != "https" || itemURL.Host == "" {
			continue
		}
		domain := normalizeDomain(firstNonEmpty(item.Domain, itemURL.Hostname()))
		if len(f.sourceSet) > 0 && !domainAllowed(domain, f.sourceSet) {
			continue
		}
		canonicalURL := itemURL.String()
		if seenURLs[canonicalURL] {
			continue
		}
		title := strings.TrimSpace(item.Title)
		if len(title) < 15 {
			continue
		}
		seenURLs[canonicalURL] = true
		articles = append(articles, Article{
			Title:         title,
			URL:           canonicalURL,
			Domain:        domain,
			PublishedAt:   normalizeGDELTTime(item.SeenDate),
			SourceCountry: strings.TrimSpace(item.SourceCountry),
			IsPrimary:     domainAllowed(domain, f.primarySet),
		})
	}
	return articles, nil
}

func retryDelay(retryAfter string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds >= 0 {
		delay := time.Duration(seconds) * time.Second
		if delay > 5*time.Second {
			return 5 * time.Second
		}
		return delay
	}
	delay := time.Duration(1<<attempt) * 500 * time.Millisecond
	if delay > 5*time.Second {
		return 5 * time.Second
	}
	return delay
}

func normalizeGDELTTime(value string) string {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"20060102T150405Z", time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
