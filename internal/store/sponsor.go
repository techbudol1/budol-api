package store

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func (s *MemgraphUserStore) CountSponsoredEscrows(ctx context.Context, owner string, since string) (int64, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (e:SponsoredTradeEscrow)
WHERE e.owner = $owner AND e.createdAt >= $since
RETURN count(e) AS count
`, map[string]any{
			"owner": strings.ToLower(strings.TrimSpace(owner)),
			"since": strings.TrimSpace(since),
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return intValue(rows.Record(), "count"), rows.Err()
		}
		return int64(0), rows.Err()
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (s *MemgraphUserStore) CountGlobalSponsoredEscrows(ctx context.Context, since string) (int64, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (e:SponsoredTradeEscrow)
WHERE e.createdAt >= $since
RETURN count(e) AS count
`, map[string]any{"since": strings.TrimSpace(since)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return intValue(rows.Record(), "count"), rows.Err()
		}
		return int64(0), rows.Err()
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (s *MemgraphUserStore) CreateSponsoredEscrow(ctx context.Context, owner string, pollID string, side string, amount float64, fee float64, total float64, txHash string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
CREATE (:SponsoredTradeEscrow {
  id: $id,
  owner: $owner,
  pollId: $pollID,
  side: $side,
  amount: $amount,
  tradingFee: $fee,
  escrowTotal: $total,
  txHash: $txHash,
  createdAt: $now
})
`, map[string]any{
			"id":     uuid.NewString(),
			"owner":  strings.ToLower(strings.TrimSpace(owner)),
			"pollID": strings.TrimSpace(pollID),
			"side":   strings.ToLower(strings.TrimSpace(side)),
			"amount": roundMoney(amount),
			"fee":    roundMoney(fee),
			"total":  roundMoney(total),
			"txHash": strings.ToLower(strings.TrimSpace(txHash)),
			"now":    now,
		})
		return nil, err
	})
	return err
}
