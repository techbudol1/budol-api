package httpapi

import (
	"testing"

	"github.com/techbudol1/budol-api/internal/config"
)

func TestShieldedPayoutPoolForAmountMatchesExactBaseUnitDenomination(t *testing.T) {
	server := Server{cfg: config.Config{
		WelcomeTokenDecimals: 18,
		ShieldedPayoutPools: []config.ShieldedPayoutPool{{
			Denomination: "19980000000000000000",
			PoolAddress:  "0x0000000000000000000000000000000000000001",
		}},
	}}

	address, denomination, ok, err := server.shieldedPayoutPoolForAmount("19.98")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || address != "0x0000000000000000000000000000000000000001" || denomination != "19980000000000000000" {
		t.Fatalf("unexpected pool match: %q %q %v", address, denomination, ok)
	}
}

func TestShieldedPayoutPoolForAmountRejectsUnsupportedAmount(t *testing.T) {
	server := Server{cfg: config.Config{
		WelcomeTokenDecimals: 18,
		ShieldedPayoutPools: []config.ShieldedPayoutPool{{
			Denomination: "100000000000000000000",
			PoolAddress:  "0x0000000000000000000000000000000000000001",
		}},
	}}

	_, _, ok, err := server.shieldedPayoutPoolForAmount("19.98")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected 19.98 BUDOL not to match a 100 BUDOL pool")
	}
}
