package httpapi

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"budol/server/internal/store"
	"budol/server/internal/thirdweb"
)

var errUnderCollateralized = errors.New("operation would make payout reserves under-collateralized")

type CollateralStatus struct {
	AvailableCollateral float64                             `json:"availableCollateral"`
	BufferBps           int                                 `json:"bufferBps"`
	Collateralized      bool                                `json:"collateralized"`
	CoveragePercent     float64                             `json:"coveragePercent"`
	Enabled             bool                                `json:"enabled"`
	FreeCollateral      float64                             `json:"freeCollateral"`
	Markets             []store.MarketCollateralRequirement `json:"markets,omitempty"`
	RequiredCollateral  float64                             `json:"requiredCollateral"`
	RequiredWithBuffer  float64                             `json:"requiredWithBuffer"`
	TokenAddress        string                              `json:"tokenAddress"`
	WalletAddress       string                              `json:"walletAddress"`
	WalletBalance       float64                             `json:"walletBalance"`
	FetchedAt           string                              `json:"fetchedAt"`
}

func (s Server) projectWalletAddress(ctx context.Context) (string, error) {
	walletAddress := strings.ToLower(strings.TrimSpace(s.cfg.ProjectWallet))
	if s.gmrEngine != nil && s.gmrEngine.Configured() {
		wallet, ok, err := s.gmrEngine.DefaultAdminWallet(ctx)
		if err != nil {
			return "", err
		}
		if ok && strings.TrimSpace(wallet.Address) != "" {
			walletAddress = strings.ToLower(strings.TrimSpace(wallet.Address))
		}
	}
	if !thirdweb.IsEVMAddress(walletAddress) {
		return "", errors.New("project escrow wallet is not configured")
	}
	return walletAddress, nil
}

func (s Server) currentCollateralStatus(ctx context.Context) (CollateralStatus, error) {
	requirement, err := s.store.CollateralRequirement(ctx)
	if err != nil {
		return CollateralStatus{}, err
	}
	return s.collateralStatusForRequirement(ctx, requirement, 0)
}

func (s Server) collateralStatusForRequirement(ctx context.Context, requirement store.CollateralRequirement, incoming float64) (CollateralStatus, error) {
	walletAddress, err := s.projectWalletAddress(ctx)
	if err != nil {
		return CollateralStatus{}, err
	}
	tokenAddress := s.activeTokenContract(ctx)
	balance, err := s.evm.ERC20Balance(ctx, tokenAddress, walletAddress, s.cfg.WelcomeTokenDecimals)
	if err != nil {
		return CollateralStatus{}, err
	}
	walletBalance, err := strconv.ParseFloat(balance.Formatted, 64)
	if err != nil {
		return CollateralStatus{}, errors.New("failed to parse project wallet balance")
	}

	required := roundCollateral(requirement.TotalRequired)
	requiredWithBuffer := collateralWithBuffer(required, s.cfg.CollateralBufferBps)
	available := roundCollateral(walletBalance + math.Max(0, incoming))
	coverage := 100.0
	if requiredWithBuffer > 0 {
		coverage = roundCollateral(available / requiredWithBuffer * 100)
	}
	return CollateralStatus{
		AvailableCollateral: available,
		BufferBps:           s.cfg.CollateralBufferBps,
		Collateralized:      !s.cfg.CollateralGuaranteeEnabled || available+0.0000001 >= requiredWithBuffer,
		CoveragePercent:     coverage,
		Enabled:             s.cfg.CollateralGuaranteeEnabled,
		FreeCollateral:      roundCollateral(walletBalance - requiredWithBuffer),
		Markets:             requirement.Markets,
		RequiredCollateral:  required,
		RequiredWithBuffer:  requiredWithBuffer,
		TokenAddress:        strings.ToLower(tokenAddress),
		WalletAddress:       walletAddress,
		WalletBalance:       roundCollateral(walletBalance),
		FetchedAt:           time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (s Server) collateralizeQuote(ctx context.Context, quote store.TradeQuote, incoming float64) (store.TradeQuote, CollateralStatus, error) {
	current, err := s.store.CollateralRequirement(ctx)
	if err != nil {
		return quote, CollateralStatus{}, err
	}
	projected := store.ProjectedCollateralRequirement(current, quote)
	status, err := s.collateralStatusForRequirement(ctx, projected, incoming)
	if err != nil {
		return quote, CollateralStatus{}, err
	}
	quote.Collateralized = status.Collateralized
	quote.CollateralRequired = status.RequiredWithBuffer
	quote.CollateralAvailable = status.AvailableCollateral
	quote.CollateralCoverage = status.CoveragePercent
	if !status.Collateralized {
		return quote, status, errUnderCollateralized
	}
	return quote, status, nil
}

func (s Server) ensureCollateralOutflow(ctx context.Context, outgoing float64, liabilityRelease float64) error {
	if !s.cfg.CollateralGuaranteeEnabled {
		return nil
	}
	if outgoing <= 0 || math.IsNaN(outgoing) || math.IsInf(outgoing, 0) {
		return errors.New("outgoing collateral must be greater than zero")
	}
	requirement, err := s.store.CollateralRequirement(ctx)
	if err != nil {
		return err
	}
	requirement.TotalRequired = math.Max(0, requirement.TotalRequired-math.Max(0, liabilityRelease))
	status, err := s.collateralStatusForRequirement(ctx, requirement, 0)
	if err != nil {
		return err
	}
	if !collateralOutflowCovered(status.WalletBalance, outgoing, requirement.TotalRequired, 0, s.cfg.CollateralBufferBps) {
		return errUnderCollateralized
	}
	return nil
}

func (s Server) ensureTokenContractCollateralized(ctx context.Context, tokenAddress string) error {
	if !s.cfg.CollateralGuaranteeEnabled {
		return nil
	}
	requirement, err := s.store.CollateralRequirement(ctx)
	if err != nil {
		return err
	}
	walletAddress, err := s.projectWalletAddress(ctx)
	if err != nil {
		return err
	}
	balance, err := s.evm.ERC20Balance(ctx, tokenAddress, walletAddress, s.cfg.WelcomeTokenDecimals)
	if err != nil {
		return err
	}
	available, err := strconv.ParseFloat(balance.Formatted, 64)
	if err != nil {
		return errors.New("failed to parse project wallet balance")
	}
	if available+0.0000001 < collateralWithBuffer(requirement.TotalRequired, s.cfg.CollateralBufferBps) {
		return errUnderCollateralized
	}
	return nil
}

func collateralWithBuffer(required float64, bufferBps int) float64 {
	if required <= 0 {
		return 0
	}
	return roundCollateral(required * (1 + float64(bufferBps)/10000))
}

func collateralOutflowCovered(walletBalance float64, outgoing float64, required float64, liabilityRelease float64, bufferBps int) bool {
	remainingRequired := math.Max(0, required-math.Max(0, liabilityRelease))
	return walletBalance-outgoing+0.0000001 >= collateralWithBuffer(remainingRequired, bufferBps)
}

func roundCollateral(value float64) float64 {
	return math.Round(value*100) / 100
}
