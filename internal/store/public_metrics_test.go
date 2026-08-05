package store

import (
	"testing"
	"time"
)

func TestAggregateMetricTradesKeepsOnlyAnonymousThirtyDayBuckets(t *testing.T) {
	now := time.Date(2026, time.August, 6, 12, 0, 0, 0, time.UTC)
	events := []metricTradeEvent{
		{CreatedAt: "2026-08-06T08:00:00Z", UserID: "user-a", Amount: 10},
		{CreatedAt: "2026-08-06T09:00:00Z", UserID: "user-a", Amount: 25},
		{CreatedAt: "2026-08-01T09:00:00Z", UserID: "user-b", Amount: 50},
		{CreatedAt: "2026-07-15T09:00:00Z", UserID: "user-c", Amount: 100},
		{CreatedAt: "2026-06-01T09:00:00Z", UserID: "too-old", Amount: 500},
	}

	seven, thirty, daily := aggregateMetricTrades(now, events)
	if seven.Trades != 3 || seven.Volume != 85 || seven.ActiveTraders != 2 {
		t.Fatalf("unexpected seven-day metrics: %+v", seven)
	}
	if thirty.Trades != 4 || thirty.Volume != 185 || thirty.ActiveTraders != 3 {
		t.Fatalf("unexpected thirty-day metrics: %+v", thirty)
	}
	if len(daily) != 30 {
		t.Fatalf("expected 30 anonymous daily buckets, got %d", len(daily))
	}
	latest := daily[len(daily)-1]
	if latest.Date != "2026-08-06" || latest.Trades != 2 || latest.Volume != 35 || latest.ActiveTraders != 1 {
		t.Fatalf("unexpected latest bucket: %+v", latest)
	}
}
