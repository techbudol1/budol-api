package store

import "testing"

func TestCollateralRequiredCoversEverySettlementPath(t *testing.T) {
	tests := []struct {
		name   string
		yes    float64
		no     float64
		refund float64
		fixed  float64
		want   float64
	}{
		{name: "yes is worst case", yes: 220, no: 180, refund: 100, want: 220},
		{name: "no is worst case", yes: 120, no: 310, refund: 200, want: 310},
		{name: "cancellation is worst case", yes: 120, no: 130, refund: 250, want: 250},
		{name: "settled unpaid claims are additive", yes: 120, no: 130, refund: 100, fixed: 75, want: 205},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := collateralRequired(test.yes, test.no, test.refund, test.fixed); got != test.want {
				t.Fatalf("collateralRequired() = %.2f, want %.2f", got, test.want)
			}
		})
	}
}

func TestProjectedCollateralRequirement(t *testing.T) {
	current := CollateralRequirement{
		Markets: []MarketCollateralRequirement{{
			PollID:             "poll-1",
			YesLiability:       100,
			NoLiability:        90,
			RefundLiability:    60,
			RequiredCollateral: 100,
		}},
		TotalRequired: 100,
	}

	projected := ProjectedCollateralRequirement(current, TradeQuote{
		PollID:          "poll-1",
		Side:            "no",
		Amount:          40,
		PotentialPayout: 80,
	})

	if got, want := projected.TotalRequired, 170.0; got != want {
		t.Fatalf("ProjectedCollateralRequirement().TotalRequired = %.2f, want %.2f", got, want)
	}
	if got, want := current.TotalRequired, 100.0; got != want {
		t.Fatalf("ProjectedCollateralRequirement mutated input: %.2f, want %.2f", got, want)
	}
}

func TestProjectedCollateralAfterCashoutOnlyReleasesReducedWorstCase(t *testing.T) {
	current := CollateralRequirement{
		Markets: []MarketCollateralRequirement{{
			PollID:             "poll-1",
			YesLiability:       200,
			NoLiability:        300,
			RefundLiability:    180,
			RequiredCollateral: 300,
		}},
		TotalRequired: 300,
	}

	projected := ProjectedCollateralAfterCashout(current, CashoutQuote{
		PollID:   "poll-1",
		Side:     "yes",
		Amount:   50,
		Shares:   80,
		Proceeds: 42,
	})

	if got, want := projected.TotalRequired, 300.0; got != want {
		t.Fatalf("cashout released collateral from the wrong worst-case side: %.2f, want %.2f", got, want)
	}
	if got := CollateralRelease(current, projected); got != 0 {
		t.Fatalf("CollateralRelease() = %.2f, want 0", got)
	}
}

func TestProjectedCollateralAfterCashoutReleasesReducedWorstCase(t *testing.T) {
	current := CollateralRequirement{
		Markets: []MarketCollateralRequirement{{
			PollID:             "poll-1",
			YesLiability:       300,
			NoLiability:        100,
			RefundLiability:    180,
			RequiredCollateral: 300,
		}},
		TotalRequired: 300,
	}

	projected := ProjectedCollateralAfterCashout(current, CashoutQuote{
		PollID: "poll-1",
		Side:   "yes",
		Amount: 50,
		Shares: 80,
	})

	if got, want := projected.TotalRequired, 220.0; got != want {
		t.Fatalf("ProjectedCollateralAfterCashout().TotalRequired = %.2f, want %.2f", got, want)
	}
	if got, want := CollateralRelease(current, projected), 80.0; got != want {
		t.Fatalf("CollateralRelease() = %.2f, want %.2f", got, want)
	}
}
