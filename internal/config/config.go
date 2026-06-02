package config

import (
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr                  string
	AppEnv                string
	AdminAPIKey           string
	AdminCookieName       string
	AdminPassword         string
	AdminUsername         string
	AllowedOrigins        []string
	ArbitrumSepoliaRPCURL string
	PublicAppURL          string
	PublicAPIURL          string
	SessionCookieName     string
	SessionSecret         string
	SessionTTL            time.Duration
	ThirdwebAPIBase       string
	ThirdwebSecretKey     string
	ThirdwebMeURL         string
	ThirdwebSendURL       string
	ProjectWallet         string
	WelcomeTokenChainID   int
	WelcomeTokenContract  string
	WelcomeTokenAmount    string
	WelcomeTokenDecimals  int
	MemgraphURI           string
	MemgraphUser          string
	MemgraphPassword      string
}

func Load() (Config, error) {
	loadDotEnv(".env", "../.env")

	cfg := Config{
		Addr:                  env("SERVER_ADDR", ":8080"),
		AppEnv:                env("APP_ENV", "development"),
		AdminAPIKey:           os.Getenv("ADMIN_API_KEY"),
		AdminCookieName:       env("ADMIN_COOKIE_NAME", "budol_admin_session"),
		AdminPassword:         env("ADMIN_PASSWORD", "MaryAnn1101"),
		AdminUsername:         env("ADMIN_USERNAME", "destrega"),
		AllowedOrigins:        splitCSV(env("ALLOWED_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000")),
		ArbitrumSepoliaRPCURL: env("ARBITRUM_SEPOLIA_RPC_URL", "https://sepolia-rollup.arbitrum.io/rpc"),
		PublicAppURL:          env("PUBLIC_APP_URL", "http://localhost:3000"),
		PublicAPIURL:          env("PUBLIC_API_URL", "http://localhost:8080"),
		SessionCookieName:     env("SESSION_COOKIE_NAME", "budol_session"),
		SessionSecret:         os.Getenv("SESSION_SECRET"),
		SessionTTL:            time.Duration(envInt("SESSION_TTL_HOURS", 24)) * time.Hour,
		ThirdwebAPIBase:       env("THIRDWEB_API_BASE", "https://api.thirdweb.com"),
		ThirdwebSecretKey:     os.Getenv("THIRDWEB_SECRET_KEY"),
		ThirdwebMeURL:         env("THIRDWEB_ME_URL", "https://api.thirdweb.com/v1/wallets/me"),
		ThirdwebSendURL:       env("THIRDWEB_SEND_URL", "https://api.thirdweb.com/v1/wallets/send"),
		ProjectWallet:         os.Getenv("THIRDWEB_PROJECT_WALLET_ADDRESS"),
		WelcomeTokenChainID:   envInt("WELCOME_TOKEN_CHAIN_ID", 421614),
		WelcomeTokenContract:  env("WELCOME_TOKEN_CONTRACT", "0x12fF5d28F93c1CABDA4Bd0ddf8906FF7E4Df1c4e"),
		WelcomeTokenAmount:    env("WELCOME_TOKEN_AMOUNT", "100"),
		WelcomeTokenDecimals:  envInt("WELCOME_TOKEN_DECIMALS", 18),
		MemgraphURI:           env("MEMGRAPH_URI", "bolt://localhost:7687"),
		MemgraphUser:          os.Getenv("MEMGRAPH_USER"),
		MemgraphPassword:      os.Getenv("MEMGRAPH_PASSWORD"),
	}

	if cfg.ThirdwebSecretKey == "" {
		return Config{}, errors.New("THIRDWEB_SECRET_KEY is required")
	}
	if len(cfg.SessionSecret) < 32 {
		return Config{}, errors.New("SESSION_SECRET must be at least 32 characters")
	}
	if len(cfg.AdminAPIKey) < 24 {
		return Config{}, errors.New("ADMIN_API_KEY must be at least 24 characters")
	}
	if cfg.AdminUsername == "" || cfg.AdminPassword == "" {
		return Config{}, errors.New("ADMIN_USERNAME and ADMIN_PASSWORD are required")
	}
	if cfg.IsProduction() {
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

func env(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
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
