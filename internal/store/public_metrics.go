package store

import (
	"context"
	"sort"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// PublicMetricsSnapshot contains aggregate product data only. It deliberately
// excludes wallet addresses, user IDs, recipients, claim notes, and proofs.
type PublicMetricsSnapshot struct {
	GeneratedAt          string             `json:"generatedAt"`
	TotalUsers           int64              `json:"totalUsers"`
	ActiveTraders        int64              `json:"activeTraders"`
	TotalTrades          int64              `json:"totalTrades"`
	TotalVolume          float64            `json:"totalVolume"`
	TotalMarkets         int64              `json:"totalMarkets"`
	OpenMarkets          int64              `json:"openMarkets"`
	ResolvedMarkets      int64              `json:"resolvedMarkets"`
	PrivateClaims        int64              `json:"privateClaims"`
	ShieldedTrades       int64              `json:"shieldedTrades"`
	ShieldedWithdrawals  int64              `json:"shieldedWithdrawals"`
	CompletedWithdrawals int64              `json:"completedWithdrawals"`
	FailedWithdrawals    int64              `json:"failedWithdrawals"`
	RecordedTransactions int64              `json:"recordedTransactions"`
	SevenDays            PublicMetricPeriod `json:"sevenDays"`
	ThirtyDays           PublicMetricPeriod `json:"thirtyDays"`
	Daily                []PublicMetricDay  `json:"daily"`
}

type PublicMetricPeriod struct {
	Trades        int64   `json:"trades"`
	Volume        float64 `json:"volume"`
	ActiveTraders int64   `json:"activeTraders"`
}

type PublicMetricDay struct {
	Date          string  `json:"date"`
	Trades        int64   `json:"trades"`
	Volume        float64 `json:"volume"`
	ActiveTraders int64   `json:"activeTraders"`
}

type metricTradeEvent struct {
	CreatedAt string
	UserID    string
	Amount    float64
}

func (s *MemgraphUserStore) PublicMetrics(ctx context.Context, now time.Time) (PublicMetricsSnapshot, error) {
	now = now.UTC()
	since30 := now.AddDate(0, 0, -29).Truncate(24 * time.Hour)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		snapshot := PublicMetricsSnapshot{GeneratedAt: now.Format(time.RFC3339), Daily: []PublicMetricDay{}}

		users, err := metricRecord(ctx, tx, `MATCH (u:User) WHERE coalesce(u.role, "user") = "user" RETURN count(u) AS totalUsers`, nil)
		if err != nil {
			return nil, err
		}
		snapshot.TotalUsers = intValue(users, "totalUsers")

		trades, err := metricRecord(ctx, tx, `
MATCH (u:User)-[:PLACED_TRADE]->(t:Trade)
RETURN count(t) AS totalTrades,
       count(DISTINCT u.id) AS activeTraders,
       coalesce(sum(t.amount), 0.0) AS totalVolume,
       count(DISTINCT CASE WHEN coalesce(t.escrowTxHash, "") <> "" THEN t.escrowTxHash ELSE null END) AS tradeTransactions
`, nil)
		if err != nil {
			return nil, err
		}
		snapshot.TotalTrades = intValue(trades, "totalTrades")
		snapshot.ActiveTraders = intValue(trades, "activeTraders")
		snapshot.TotalVolume = roundMoney(floatValue(trades, "totalVolume"))
		snapshot.RecordedTransactions = intValue(trades, "tradeTransactions")

		markets, err := metricRecord(ctx, tx, `
MATCH (p:Poll)
WHERE coalesce(p.visibility, "public") = "public"
RETURN count(p) AS totalMarkets,
       sum(CASE WHEN coalesce(p.status, "") = "published" THEN 1 ELSE 0 END) AS openMarkets,
       sum(CASE WHEN coalesce(p.status, "") IN ["resolved", "cancelled"] THEN 1 ELSE 0 END) AS resolvedMarkets
`, nil)
		if err != nil {
			return nil, err
		}
		snapshot.TotalMarkets = intValue(markets, "totalMarkets")
		snapshot.OpenMarkets = intValue(markets, "openMarkets")
		snapshot.ResolvedMarkets = intValue(markets, "resolvedMarkets")

		privacy, err := metricRecord(ctx, tx, `
OPTIONAL MATCH (claim:PrivateClaim)
WITH count(claim) AS privateClaims
OPTIONAL MATCH (order:ShieldedTradeOrder)
WITH privateClaims, count(order) AS shieldedTrades
OPTIONAL MATCH (withdrawal:ShieldedWithdrawal)
RETURN privateClaims,
       shieldedTrades,
       count(withdrawal) AS shieldedWithdrawals,
       sum(CASE WHEN coalesce(withdrawal.status, "") = "confirmed" THEN 1 ELSE 0 END) AS completedWithdrawals,
       sum(CASE WHEN coalesce(withdrawal.status, "") IN ["failed", "suspicious"] THEN 1 ELSE 0 END) AS failedWithdrawals,
       count(DISTINCT CASE WHEN coalesce(withdrawal.transactionHash, "") <> "" THEN withdrawal.transactionHash ELSE null END) AS withdrawalTransactions
`, nil)
		if err != nil {
			return nil, err
		}
		snapshot.PrivateClaims = intValue(privacy, "privateClaims")
		snapshot.ShieldedTrades = intValue(privacy, "shieldedTrades")
		snapshot.ShieldedWithdrawals = intValue(privacy, "shieldedWithdrawals")
		snapshot.CompletedWithdrawals = intValue(privacy, "completedWithdrawals")
		snapshot.FailedWithdrawals = intValue(privacy, "failedWithdrawals")
		snapshot.RecordedTransactions += intValue(privacy, "withdrawalTransactions")

		events, err := metricTradeEvents(ctx, tx, since30)
		if err != nil {
			return nil, err
		}
		snapshot.SevenDays, snapshot.ThirtyDays, snapshot.Daily = aggregateMetricTrades(now, events)
		return snapshot, nil
	})
	if err != nil {
		return PublicMetricsSnapshot{}, err
	}
	return result.(PublicMetricsSnapshot), nil
}

func metricRecord(ctx context.Context, tx neo4j.ManagedTransaction, query string, params map[string]any) (*neo4j.Record, error) {
	rows, err := tx.Run(ctx, query, params)
	if err != nil {
		return nil, err
	}
	if rows.Next(ctx) {
		record := rows.Record()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return record, nil
	}
	return &neo4j.Record{}, rows.Err()
}

func metricTradeEvents(ctx context.Context, tx neo4j.ManagedTransaction, since time.Time) ([]metricTradeEvent, error) {
	rows, err := tx.Run(ctx, `
MATCH (u:User)-[:PLACED_TRADE]->(t:Trade)
WHERE coalesce(t.createdAt, "") >= $since
RETURN t.createdAt AS createdAt, u.id AS userId, coalesce(t.amount, 0.0) AS amount
ORDER BY t.createdAt ASC
`, map[string]any{"since": since.Format(time.RFC3339)})
	if err != nil {
		return nil, err
	}
	events := []metricTradeEvent{}
	for rows.Next(ctx) {
		record := rows.Record()
		events = append(events, metricTradeEvent{CreatedAt: stringValue(record, "createdAt"), UserID: stringValue(record, "userId"), Amount: floatValue(record, "amount")})
	}
	return events, rows.Err()
}

func aggregateMetricTrades(now time.Time, events []metricTradeEvent) (PublicMetricPeriod, PublicMetricPeriod, []PublicMetricDay) {
	now = now.UTC()
	start30 := now.AddDate(0, 0, -29).Truncate(24 * time.Hour)
	start7 := now.AddDate(0, 0, -6).Truncate(24 * time.Hour)
	type dailyAccumulator struct {
		trades int64
		volume float64
		users  map[string]struct{}
	}
	days := map[string]*dailyAccumulator{}
	period7Users := map[string]struct{}{}
	period30Users := map[string]struct{}{}
	seven := PublicMetricPeriod{}
	thirty := PublicMetricPeriod{}
	for index := 0; index < 30; index++ {
		day := start30.AddDate(0, 0, index).Format("2006-01-02")
		days[day] = &dailyAccumulator{users: map[string]struct{}{}}
	}
	for _, event := range events {
		createdAt, err := time.Parse(time.RFC3339, event.CreatedAt)
		if err != nil {
			continue
		}
		createdAt = createdAt.UTC()
		if createdAt.Before(start30) || createdAt.After(now) {
			continue
		}
		day := createdAt.Format("2006-01-02")
		bucket, ok := days[day]
		if !ok {
			continue
		}
		bucket.trades++
		bucket.volume += event.Amount
		bucket.users[event.UserID] = struct{}{}
		thirty.Trades++
		thirty.Volume += event.Amount
		period30Users[event.UserID] = struct{}{}
		if !createdAt.Before(start7) {
			seven.Trades++
			seven.Volume += event.Amount
			period7Users[event.UserID] = struct{}{}
		}
	}
	seven.Volume = roundMoney(seven.Volume)
	seven.ActiveTraders = int64(len(period7Users))
	thirty.Volume = roundMoney(thirty.Volume)
	thirty.ActiveTraders = int64(len(period30Users))
	daily := make([]PublicMetricDay, 0, len(days))
	for date, bucket := range days {
		daily = append(daily, PublicMetricDay{Date: date, Trades: bucket.trades, Volume: roundMoney(bucket.volume), ActiveTraders: int64(len(bucket.users))})
	}
	sort.Slice(daily, func(i, j int) bool { return daily[i].Date < daily[j].Date })
	return seven, thirty, daily
}
