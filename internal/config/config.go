package config

import (
	"errors"
	"math/big"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr                               string
	AppEnv                             string
	AdminAPIKey                        string
	AdminCookieName                    string
	AdminPassword                      string
	AdminUsername                      string
	AllowedOrigins                     []string
	ArbitrumSepoliaRPCURL              string
	CollateralBufferBps                int
	CollateralGuaranteeEnabled         bool
	PublicAppURL                       string
	PublicAPIURL                       string
	PrivyAPIBase                       string
	PrivyAppID                         string
	PrivyAppSecret                     string
	PrivyVerificationKey               string
	SessionCookieName                  string
	SessionSecret                      string
	SessionTTL                         time.Duration
	SeedDemoPolls                      bool
	ThirdwebAPIBase                    string
	ThirdwebSecretKey                  string
	ThirdwebMeURL                      string
	ThirdwebSendURL                    string
	GMREngineAPIBase                   string
	GMREngineAPIKey                    string
	FacebookOAuthClientID              string
	FacebookOAuthClientSecret          string
	FacebookOAuthRedirectURL           string
	GoogleOAuthClientID                string
	GoogleOAuthClientSecret            string
	GoogleOAuthRedirectURL             string
	ProjectWallet                      string
	PrivateClaimRegistryAddress        string
	PrivateClaimRegistryChainID        int
	PrivateClaimRegistryRequired       bool
	PrivateClaimRegistryConfirmTimeout time.Duration
	ShieldedPayoutEnabled              bool
	ShieldedPayoutPoolAddress          string
	ShieldedPayoutPoolChainID          int
	ShieldedPayoutDenomination         string
	ShieldedPayoutPools                []ShieldedPayoutPool
	ShieldedPayoutRequired             bool
	ShieldedPayoutDirectFallback       bool
	ShieldedPayoutConfirmTimeout       time.Duration
	ShieldedWithdrawalBatchLimit       int
	ShieldedWithdrawalBaseBackoff      time.Duration
	ShieldedWithdrawalDelay            time.Duration
	ShieldedWithdrawalMaxAttempts      int
	ShieldedWithdrawalPollInterval     time.Duration
	ShieldedWithdrawalRelayerAddress   string
	ShieldedWithdrawalRelayerFee       string
	ShieldedWithdrawalStaleProcessing  time.Duration
	ShieldedWithdrawalMode             string
	WelcomeTokenChainID                int
	WelcomeTokenContract               string
	WelcomeTokenAmount                 string
	WelcomeTokenDecimals               int
	MemgraphURI                        string
	MemgraphUser                       string
	MemgraphPassword                   string
	NewsAgentEnabled                   bool
	NewsAgentGDELTURL                  string
	NewsAgentRSSURLs                   []string
	NewsAgentInterval                  time.Duration
	NewsAgentMaxArticles               int
	NewsAgentMaxCandidates             int
	NewsAgentPrimaryDomains            []string
	NewsAgentQuery                     string
	NewsAgentSourceDomains             []string
	OpenAIAPIKey                       string
	OpenAIModel                        string
}

type ShieldedPayoutPool struct {
	Denomination string
	PoolAddress  string
}

func Load() (Config, error) {
	loadDotEnv(".env.server", "../.env.server")

	cfg := Config{
		Addr:                               env("SERVER_ADDR", ":8080"),
		AppEnv:                             env("APP_ENV", "development"),
		AdminAPIKey:                        os.Getenv("ADMIN_API_KEY"),
		AdminCookieName:                    env("ADMIN_COOKIE_NAME", "budol_admin_session"),
		AdminPassword:                      env("ADMIN_PASSWORD", "MaryAnn1101"),
		AdminUsername:                      env("ADMIN_USERNAME", "destrega"),
		AllowedOrigins:                     splitCSV(env("ALLOWED_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000")),
		ArbitrumSepoliaRPCURL:              env("ARBITRUM_SEPOLIA_RPC_URL", "https://sepolia-rollup.arbitrum.io/rpc"),
		CollateralBufferBps:                envNonNegativeInt("COLLATERAL_BUFFER_BPS", 0),
		CollateralGuaranteeEnabled:         envBool("COLLATERAL_GUARANTEE_ENABLED", true),
		PrivyAPIBase:                       env("PRIVY_API_BASE", "https://api.privy.io"),
		PrivyAppID:                         os.Getenv("PRIVY_APP_ID"),
		PrivyAppSecret:                     os.Getenv("PRIVY_APP_SECRET"),
		PrivyVerificationKey:               os.Getenv("PRIVY_VERIFICATION_KEY"),
		PublicAppURL:                       env("PUBLIC_APP_URL", "http://localhost:3000"),
		PublicAPIURL:                       env("PUBLIC_API_URL", "http://localhost:8080"),
		SessionCookieName:                  env("SESSION_COOKIE_NAME", "budol_session"),
		SessionSecret:                      os.Getenv("SESSION_SECRET"),
		SessionTTL:                         time.Duration(envInt("SESSION_TTL_HOURS", 24)) * time.Hour,
		SeedDemoPolls:                      envBool("SEED_DEMO_POLLS", false),
		ThirdwebAPIBase:                    env("THIRDWEB_API_BASE", "https://api.thirdweb.com"),
		ThirdwebSecretKey:                  os.Getenv("THIRDWEB_SECRET_KEY"),
		ThirdwebMeURL:                      env("THIRDWEB_ME_URL", "https://api.thirdweb.com/v1/wallets/me"),
		ThirdwebSendURL:                    env("THIRDWEB_SEND_URL", "https://api.thirdweb.com/v1/wallets/send"),
		GMREngineAPIBase:                   env("GMR_ENGINE_API_BASE", "http://localhost:8090"),
		GMREngineAPIKey:                    os.Getenv("GMR_ENGINE_API_KEY"),
		FacebookOAuthClientID:              os.Getenv("FACEBOOK_OAUTH_CLIENT_ID"),
		FacebookOAuthClientSecret:          os.Getenv("FACEBOOK_OAUTH_CLIENT_SECRET"),
		FacebookOAuthRedirectURL:           env("FACEBOOK_OAUTH_REDIRECT_URL", env("PUBLIC_API_URL", "http://localhost:8080")+"/api/auth/facebook/callback"),
		GoogleOAuthClientID:                os.Getenv("GOOGLE_OAUTH_CLIENT_ID"),
		GoogleOAuthClientSecret:            os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET"),
		GoogleOAuthRedirectURL:             env("GOOGLE_OAUTH_REDIRECT_URL", env("PUBLIC_API_URL", "http://localhost:8080")+"/api/auth/google/callback"),
		ProjectWallet:                      firstEnv("BUDOL_PROJECT_WALLET_ADDRESS", "THIRDWEB_PROJECT_WALLET_ADDRESS", "SERVER_VAULT_WALLET_ADDRESS"),
		PrivateClaimRegistryAddress:        os.Getenv("PRIVATE_CLAIM_REGISTRY_ADDRESS"),
		PrivateClaimRegistryChainID:        envInt("PRIVATE_CLAIM_REGISTRY_CHAIN_ID", envInt("WELCOME_TOKEN_CHAIN_ID", 421614)),
		PrivateClaimRegistryRequired:       envBool("PRIVATE_CLAIM_REGISTRY_REQUIRED", false),
		PrivateClaimRegistryConfirmTimeout: time.Duration(envInt("PRIVATE_CLAIM_REGISTRY_CONFIRM_TIMEOUT_SECONDS", 90)) * time.Second,
		ShieldedPayoutEnabled:              envBool("SHIELDED_PAYOUT_ENABLED", false),
		ShieldedPayoutPoolAddress:          os.Getenv("SHIELDED_PAYOUT_POOL_ADDRESS"),
		ShieldedPayoutPoolChainID:          envInt("SHIELDED_PAYOUT_POOL_CHAIN_ID", envInt("WELCOME_TOKEN_CHAIN_ID", 421614)),
		ShieldedPayoutDenomination:         os.Getenv("SHIELDED_PAYOUT_DENOMINATION"),
		ShieldedPayoutRequired:             envBool("SHIELDED_PAYOUT_REQUIRED", false),
		ShieldedPayoutDirectFallback:       envBool("SHIELDED_PAYOUT_DIRECT_FALLBACK", false),
		ShieldedPayoutConfirmTimeout:       time.Duration(envInt("SHIELDED_PAYOUT_CONFIRM_TIMEOUT_SECONDS", 90)) * time.Second,
		ShieldedWithdrawalBatchLimit:       envInt("SHIELDED_WITHDRAWAL_BATCH_LIMIT", 5),
		ShieldedWithdrawalBaseBackoff:      time.Duration(envInt("SHIELDED_WITHDRAWAL_BASE_BACKOFF_SECONDS", 120)) * time.Second,
		ShieldedWithdrawalDelay:            time.Duration(envInt("SHIELDED_WITHDRAWAL_DELAY_SECONDS", 600)) * time.Second,
		ShieldedWithdrawalMaxAttempts:      envInt("SHIELDED_WITHDRAWAL_MAX_ATTEMPTS", 3),
		ShieldedWithdrawalPollInterval:     time.Duration(envInt("SHIELDED_WITHDRAWAL_POLL_INTERVAL_SECONDS", 15)) * time.Second,
		ShieldedWithdrawalRelayerAddress:   os.Getenv("SHIELDED_WITHDRAWAL_RELAYER_ADDRESS"),
		ShieldedWithdrawalRelayerFee:       env("SHIELDED_WITHDRAWAL_RELAYER_FEE", "0"),
		ShieldedWithdrawalStaleProcessing:  time.Duration(envInt("SHIELDED_WITHDRAWAL_STALE_PROCESSING_SECONDS", 600)) * time.Second,
		ShieldedWithdrawalMode:             strings.ToLower(env("SHIELDED_WITHDRAWAL_MODE", "zkverify")),
		WelcomeTokenChainID:                envInt("WELCOME_TOKEN_CHAIN_ID", 421614),
		WelcomeTokenContract:               env("WELCOME_TOKEN_CONTRACT", "0x689513fb392e460c6d9225f911fce57fe50d6db4"),
		WelcomeTokenAmount:                 env("WELCOME_TOKEN_AMOUNT", "100"),
		WelcomeTokenDecimals:               envInt("WELCOME_TOKEN_DECIMALS", 18),
		MemgraphURI:                        env("MEMGRAPH_URI", "bolt://localhost:7687"),
		MemgraphUser:                       os.Getenv("MEMGRAPH_USER"),
		MemgraphPassword:                   os.Getenv("MEMGRAPH_PASSWORD"),
		NewsAgentEnabled:                   envBool("NEWS_AGENT_ENABLED", true),
		NewsAgentGDELTURL:                  env("NEWS_AGENT_GDELT_URL", "https://api.gdeltproject.org/api/v2/doc/doc"),
		NewsAgentRSSURLs:                   splitCSV(env("NEWS_AGENT_RSS_URLS", "https://newsinfo.inquirer.net/feed,https://www.philstar.com/rss/headlines,https://www.rappler.com/feed/")),
		NewsAgentInterval:                  time.Duration(envInt("NEWS_AGENT_INTERVAL_MINUTES", 15)) * time.Minute,
		NewsAgentMaxArticles:               envInt("NEWS_AGENT_MAX_ARTICLES", 40),
		NewsAgentMaxCandidates:             envInt("NEWS_AGENT_MAX_CANDIDATES", 5),
		NewsAgentPrimaryDomains:            splitCSV(env("NEWS_AGENT_PRIMARY_DOMAINS", "officialgazette.gov.ph,comelec.gov.ph,pagasa.dost.gov.ph,psa.gov.ph,bsp.gov.ph,pse.com.ph")),
		NewsAgentQuery:                     env("NEWS_AGENT_QUERY", "Philippines OR Filipino OR Manila"),
		NewsAgentSourceDomains:             splitCSV(env("NEWS_AGENT_SOURCE_DOMAINS", "gmanetwork.com,abs-cbn.com,inquirer.net,rappler.com,philstar.com,bworldonline.com,pna.gov.ph,officialgazette.gov.ph,comelec.gov.ph,pagasa.dost.gov.ph,psa.gov.ph,bsp.gov.ph,pse.com.ph")),
		OpenAIAPIKey:                       strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		OpenAIModel:                        env("OPENAI_MODEL", "gpt-5.5"),
	}
	cfg.ShieldedPayoutPools = parseShieldedPayoutPools(os.Getenv("SHIELDED_PAYOUT_POOLS"), cfg.ShieldedPayoutDenomination, cfg.ShieldedPayoutPoolAddress)

	if len(cfg.SessionSecret) < 32 {
		return Config{}, errors.New("SESSION_SECRET must be at least 32 characters")
	}
	if len(cfg.AdminAPIKey) < 24 {
		return Config{}, errors.New("ADMIN_API_KEY must be at least 24 characters")
	}
	if cfg.AdminUsername == "" || cfg.AdminPassword == "" {
		return Config{}, errors.New("ADMIN_USERNAME and ADMIN_PASSWORD are required")
	}
	if cfg.CollateralBufferBps < 0 || cfg.CollateralBufferBps > 10000 {
		return Config{}, errors.New("COLLATERAL_BUFFER_BPS must be between 0 and 10000")
	}
	if cfg.ShieldedPayoutRequired && !cfg.ShieldedPayoutEnabled {
		return Config{}, errors.New("SHIELDED_PAYOUT_REQUIRED=true requires SHIELDED_PAYOUT_ENABLED=true")
	}
	if cfg.PrivateClaimRegistryRequired {
		if !isEVMAddress(cfg.PrivateClaimRegistryAddress) {
			return Config{}, errors.New("PRIVATE_CLAIM_REGISTRY_ADDRESS must be a valid EVM address when registry is required")
		}
		if cfg.PrivateClaimRegistryChainID <= 0 {
			return Config{}, errors.New("PRIVATE_CLAIM_REGISTRY_CHAIN_ID must be greater than zero")
		}
	}
	if cfg.ShieldedPayoutEnabled {
		if err := validateShieldedPayoutConfig(cfg); err != nil {
			return Config{}, err
		}
	}
	if cfg.IsProduction() {
		if !cfg.CollateralGuaranteeEnabled {
			return Config{}, errors.New("COLLATERAL_GUARANTEE_ENABLED must be true in production")
		}
		if len(cfg.AllowedOrigins) == 0 {
			return Config{}, errors.New("ALLOWED_ORIGINS is required in production")
		}
		for _, origin := range cfg.AllowedOrigins {
			if origin == "*" {
				return Config{}, errors.New("ALLOWED_ORIGINS cannot contain * in production")
			}
		}
		if cfg.AdminUsername == "destrega" || cfg.AdminPassword == "MaryAnn1101" {
			return Config{}, errors.New("change seeded admin credentials before production")
		}
		if strings.Contains(cfg.PublicAppURL, "localhost") || strings.Contains(cfg.PublicAPIURL, "localhost") {
			return Config{}, errors.New("PUBLIC_APP_URL and PUBLIC_API_URL must not use localhost in production")
		}
		if cfg.ShieldedPayoutEnabled && !cfg.ShieldedPayoutRequired {
			return Config{}, errors.New("SHIELDED_PAYOUT_REQUIRED must be true when shielded payout is enabled in production")
		}
		if cfg.ShieldedPayoutEnabled && cfg.ShieldedWithdrawalMode != "verified" {
			return Config{}, errors.New("SHIELDED_WITHDRAWAL_MODE must be verified when shielded payout is enabled in production")
		}
		if cfg.PrivateClaimRegistryAddress != "" && !cfg.PrivateClaimRegistryRequired {
			return Config{}, errors.New("PRIVATE_CLAIM_REGISTRY_REQUIRED must be true when private claim registry is configured in production")
		}
	}
	if _, err := url.ParseRequestURI(cfg.PublicAppURL); err != nil {
		return Config{}, errors.New("PUBLIC_APP_URL must be a valid URL")
	}
	if _, err := url.ParseRequestURI(cfg.PublicAPIURL); err != nil {
		return Config{}, errors.New("PUBLIC_API_URL must be a valid URL")
	}

	return cfg, nil
}

func (c Config) IsProduction() bool {
	return c.AppEnv == "production"
}

func validateShieldedPayoutConfig(cfg Config) error {
	if cfg.ShieldedWithdrawalMode != "zkverify" && cfg.ShieldedWithdrawalMode != "verified" {
		return errors.New("SHIELDED_WITHDRAWAL_MODE must be zkverify or verified")
	}
	if cfg.GMREngineAPIBase == "" || cfg.GMREngineAPIKey == "" {
		return errors.New("GMR_ENGINE_API_BASE and GMR_ENGINE_API_KEY are required for shielded payouts")
	}
	if len(cfg.ShieldedPayoutPools) == 0 {
		return errors.New("SHIELDED_PAYOUT_POOLS or SHIELDED_PAYOUT_DENOMINATION/SHIELDED_PAYOUT_POOL_ADDRESS is required")
	}
	if strings.TrimSpace(cfg.ShieldedPayoutPoolAddress) != "" && !isEVMAddress(cfg.ShieldedPayoutPoolAddress) {
		return errors.New("SHIELDED_PAYOUT_POOL_ADDRESS must be a valid EVM address")
	}
	if cfg.ShieldedPayoutPoolChainID <= 0 {
		return errors.New("SHIELDED_PAYOUT_POOL_CHAIN_ID must be greater than zero")
	}
	if cfg.ShieldedPayoutPoolChainID != cfg.WelcomeTokenChainID {
		return errors.New("SHIELDED_PAYOUT_POOL_CHAIN_ID must match WELCOME_TOKEN_CHAIN_ID")
	}
	if strings.TrimSpace(cfg.ShieldedPayoutDenomination) != "" && !isPositiveInteger(cfg.ShieldedPayoutDenomination) {
		return errors.New("SHIELDED_PAYOUT_DENOMINATION must be a positive integer in token base units")
	}
	for _, pool := range cfg.ShieldedPayoutPools {
		if !isPositiveInteger(pool.Denomination) {
			return errors.New("SHIELDED_PAYOUT_POOLS denominations must be positive base-unit integers")
		}
		if !isEVMAddress(pool.PoolAddress) {
			return errors.New("SHIELDED_PAYOUT_POOLS pool addresses must be valid EVM addresses")
		}
	}
	if !isEVMAddress(cfg.WelcomeTokenContract) {
		return errors.New("WELCOME_TOKEN_CONTRACT must be a valid EVM address for shielded payouts")
	}
	if cfg.ShieldedWithdrawalBatchLimit <= 0 {
		return errors.New("SHIELDED_WITHDRAWAL_BATCH_LIMIT must be greater than zero")
	}
	if cfg.ShieldedWithdrawalBaseBackoff <= 0 {
		return errors.New("SHIELDED_WITHDRAWAL_BASE_BACKOFF_SECONDS must be greater than zero")
	}
	if cfg.ShieldedWithdrawalDelay < 0 {
		return errors.New("SHIELDED_WITHDRAWAL_DELAY_SECONDS must be zero or greater")
	}
	if cfg.ShieldedWithdrawalMaxAttempts <= 0 {
		return errors.New("SHIELDED_WITHDRAWAL_MAX_ATTEMPTS must be greater than zero")
	}
	if cfg.ShieldedWithdrawalPollInterval <= 0 {
		return errors.New("SHIELDED_WITHDRAWAL_POLL_INTERVAL_SECONDS must be greater than zero")
	}
	if cfg.ShieldedWithdrawalStaleProcessing <= 0 {
		return errors.New("SHIELDED_WITHDRAWAL_STALE_PROCESSING_SECONDS must be greater than zero")
	}
	if strings.TrimSpace(cfg.ShieldedWithdrawalRelayerAddress) != "" && !isEVMAddress(cfg.ShieldedWithdrawalRelayerAddress) {
		return errors.New("SHIELDED_WITHDRAWAL_RELAYER_ADDRESS must be a valid EVM address")
	}
	if !isNonNegativeInteger(cfg.ShieldedWithdrawalRelayerFee) {
		return errors.New("SHIELDED_WITHDRAWAL_RELAYER_FEE must be a non-negative integer in token base units")
	}
	return nil
}

func parseShieldedPayoutPools(raw string, fallbackDenomination string, fallbackPool string) []ShieldedPayoutPool {
	pools := []ShieldedPayoutPool{}
	for _, item := range splitCSV(raw) {
		parts := strings.Split(item, ":")
		if len(parts) != 2 {
			continue
		}
		denomination := strings.TrimSpace(parts[0])
		poolAddress := strings.TrimSpace(parts[1])
		if denomination == "" || poolAddress == "" {
			continue
		}
		pools = append(pools, ShieldedPayoutPool{
			Denomination: denomination,
			PoolAddress:  strings.ToLower(poolAddress),
		})
	}
	if len(pools) == 0 && strings.TrimSpace(fallbackDenomination) != "" && strings.TrimSpace(fallbackPool) != "" {
		pools = append(pools, ShieldedPayoutPool{
			Denomination: strings.TrimSpace(fallbackDenomination),
			PoolAddress:  strings.ToLower(strings.TrimSpace(fallbackPool)),
		})
	}
	return pools
}

func isEVMAddress(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 42 || !strings.HasPrefix(value, "0x") {
		return false
	}
	for _, char := range value[2:] {
		if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F') {
			continue
		}
		return false
	}
	return true
}

func isPositiveInteger(value string) bool {
	number, ok := new(big.Int).SetString(strings.TrimSpace(value), 10)
	return ok && number.Sign() > 0
}

func isNonNegativeInteger(value string) bool {
	number, ok := new(big.Int).SetString(strings.TrimSpace(value), 10)
	return ok && number.Sign() >= 0
}

func env(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envNonNegativeInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
