package httpapi

import (
	"math"
	"time"

	"github.com/gofiber/fiber/v2"
)

func (s Server) publicMetrics(c *fiber.Ctx) error {
	snapshot, err := s.store.PublicMetrics(c.Context(), time.Now().UTC())
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load public testnet metrics")
	}
	decidedWithdrawals := snapshot.CompletedWithdrawals + snapshot.FailedWithdrawals
	withdrawalSuccessRate := 0.0
	if decidedWithdrawals > 0 {
		withdrawalSuccessRate = math.Round((float64(snapshot.CompletedWithdrawals)/float64(decidedWithdrawals))*1000) / 10
	}
	c.Set("Cache-Control", "public, max-age=30, stale-while-revalidate=60")
	c.Response().Header.Del("Pragma")
	return c.JSON(fiber.Map{
		"metrics": fiber.Map{
			"generatedAt": snapshot.GeneratedAt,
			"network": fiber.Map{
				"chainId": s.cfg.WelcomeTokenChainID,
				"name":    networkName(s.cfg.WelcomeTokenChainID),
				"stage":   "testnet",
			},
			"totals": fiber.Map{
				"users":                snapshot.TotalUsers,
				"activeTraders":        snapshot.ActiveTraders,
				"trades":               snapshot.TotalTrades,
				"volume":               snapshot.TotalVolume,
				"markets":              snapshot.TotalMarkets,
				"openMarkets":          snapshot.OpenMarkets,
				"resolvedMarkets":      snapshot.ResolvedMarkets,
				"recordedTransactions": snapshot.RecordedTransactions,
			},
			"activity": fiber.Map{
				"sevenDays":  snapshot.SevenDays,
				"thirtyDays": snapshot.ThirtyDays,
				"daily":      snapshot.Daily,
			},
			"privacy": fiber.Map{
				"privateClaims":         snapshot.PrivateClaims,
				"shieldedTrades":        snapshot.ShieldedTrades,
				"shieldedWithdrawals":   snapshot.ShieldedWithdrawals,
				"completedWithdrawals":  snapshot.CompletedWithdrawals,
				"failedWithdrawals":     snapshot.FailedWithdrawals,
				"withdrawalSuccessRate": withdrawalSuccessRate,
			},
			"pilot":       snapshot.Pilot,
			"privacyNote": "All metrics are aggregated. Wallet addresses, recipients, claim notes, proofs, and individual activity are excluded.",
		},
	})
}
