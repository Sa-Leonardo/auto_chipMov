package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppAddr                  string
	AdminKey                 string
	Easy2UseBaseURL          string
	Easy2UseUserToken        string
	AllowedCNPJs             []string
	DatabaseURL              string
	JWTSecret                string
	AccessTokenTTL           time.Duration
	RefreshTokenTTL          time.Duration
	BootstrapAdminEmail      string
	BootstrapAdminPassword   string
	BootstrapAdminName       string
	CORSAllowedOrigins       []string
	RechargeIntervalMonths   int
	RechargeSafetyWindowDays int
	DefaultRechargeQuantity  int
	ProviderRequestDelay     time.Duration
	EnableRealRecharge       bool
	EnableDevRoutes          bool
}

func Load() (Config, error) {
	_ = loadDotEnv(".env")

	cfg := Config{
		AppAddr:                  getEnv("APP_ADDR", ":8080"),
		AdminKey:                 os.Getenv("ADMIN_KEY"),
		Easy2UseBaseURL:          strings.TrimRight(getEnv("EASY2USE_BASE_URL", "https://mvno.tipbrasil.com.br/api/public"), "/"),
		Easy2UseUserToken:        os.Getenv("EASY2USE_USER_TOKEN"),
		AllowedCNPJs:             parseCSV(os.Getenv("ALLOWED_CNPJS")),
		DatabaseURL:              os.Getenv("DATABASE_URL"),
		JWTSecret:                os.Getenv("JWT_SECRET"),
		AccessTokenTTL:           time.Duration(getEnvInt("ACCESS_TOKEN_TTL_MINUTES", 30)) * time.Minute,
		RefreshTokenTTL:          time.Duration(getEnvInt("REFRESH_TOKEN_TTL_DAYS", 7)) * 24 * time.Hour,
		BootstrapAdminEmail:      strings.ToLower(strings.TrimSpace(getEnv("BOOTSTRAP_ADMIN_EMAIL", "admin@chipmov.local"))),
		BootstrapAdminPassword:   os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"),
		BootstrapAdminName:       getEnv("BOOTSTRAP_ADMIN_NAME", "Administrador chip-MOV"),
		CORSAllowedOrigins:       parseStringCSV(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:8080")),
		RechargeIntervalMonths:   getEnvInt("RECHARGE_INTERVAL_MONTHS", 11),
		RechargeSafetyWindowDays: getEnvInt("RECHARGE_SAFETY_WINDOW_DAYS", 10),
		DefaultRechargeQuantity:  getEnvInt("DEFAULT_RECHARGE_QUANTITY", 1),
		ProviderRequestDelay:     time.Duration(getEnvInt("PROVIDER_REQUEST_DELAY_MS", 1200)) * time.Millisecond,
		EnableRealRecharge:       getEnvBool("ENABLE_REAL_RECHARGE", false),
		EnableDevRoutes:          getEnvBool("ENABLE_DEV_ROUTES", false),
	}

	if cfg.AdminKey == "" {
		return Config{}, errors.New("ADMIN_KEY is required")
	}
	if cfg.Easy2UseUserToken == "" {
		return Config{}, errors.New("EASY2USE_USER_TOKEN is required")
	}
	if len(cfg.AllowedCNPJs) == 0 {
		return Config{}, errors.New("ALLOWED_CNPJS is required")
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return Config{}, errors.New("JWT_SECRET is required")
	}
	if cfg.BootstrapAdminPassword == "" {
		return Config{}, errors.New("BOOTSTRAP_ADMIN_PASSWORD is required")
	}
	if cfg.RechargeIntervalMonths <= 0 {
		return Config{}, errors.New("RECHARGE_INTERVAL_MONTHS must be greater than zero")
	}
	if cfg.RechargeSafetyWindowDays < 0 {
		return Config{}, errors.New("RECHARGE_SAFETY_WINDOW_DAYS cannot be negative")
	}
	if cfg.DefaultRechargeQuantity < 1 {
		return Config{}, errors.New("DEFAULT_RECHARGE_QUANTITY must be at least 1")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "sim"
}

func parseCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		clean := OnlyDigits(part)
		if clean != "" {
			out = append(out, clean)
		}
	}
	return out
}

func parseStringCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		clean := strings.TrimSpace(part)
		if clean != "" {
			out = append(out, clean)
		}
	}
	return out
}

func OnlyDigits(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("set env %s: %w", key, err)
			}
		}
	}
	return scanner.Err()
}
