// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"os"
	"time"
)

// Config holds the environment-derived settings the API needs to start.
type Config struct {
	MongoURI             string
	MongoDatabase        string
	JWTSecret            string
	JWTTTL               time.Duration
	HTTPPort             string
	GRPCPort             string
	UserCountLogInterval time.Duration
}

// Load reads Config from the environment, applying defaults for optional
// values and failing if a required one is missing or malformed.
func Load() (Config, error) {
	cfg := Config{
		MongoURI:      os.Getenv("MONGO_URI"),
		MongoDatabase: getEnvDefault("MONGO_DATABASE", "user_management"),
		JWTSecret:     os.Getenv("JWT_SECRET"),
		HTTPPort:      getEnvDefault("HTTP_PORT", "8080"),
		GRPCPort:      getEnvDefault("GRPC_PORT", "9090"),
	}

	if cfg.MongoURI == "" {
		return Config{}, errors.New("MONGO_URI is required")
	}
	if cfg.JWTSecret == "" {
		return Config{}, errors.New("JWT_SECRET is required")
	}

	jwtTTL, err := parseDurationDefault("JWT_TTL", 24*time.Hour)
	if err != nil {
		return Config{}, err
	}
	cfg.JWTTTL = jwtTTL

	reportInterval, err := parseDurationDefault("USER_COUNT_LOG_INTERVAL", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	cfg.UserCountLogInterval = reportInterval

	return cfg, nil
}

func getEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseDurationDefault(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	return time.ParseDuration(v)
}
