package httpapi

import "testing"

func TestCollateralOutflowCoveredAllowsBackedPayoutRelease(t *testing.T) {
	if !collateralOutflowCovered(100, 80, 100, 80, 0) {
		t.Fatal("expected a payout that releases the same liability to remain covered")
	}
}

func TestCollateralOutflowCoveredRejectsTreasurySpendFromReserve(t *testing.T) {
	if collateralOutflowCovered(100, 1, 100, 0, 0) {
		t.Fatal("expected treasury spend from reserved collateral to be rejected")
	}
}

func TestCollateralOutflowCoveredPreservesBuffer(t *testing.T) {
	if collateralOutflowCovered(110, 1, 100, 0, 1000) {
		t.Fatal("expected transfer to preserve the configured ten-percent collateral buffer")
	}
	if !collateralOutflowCovered(111, 1, 100, 0, 1000) {
		t.Fatal("expected transfer from collateral above the buffer to be allowed")
	}
}
