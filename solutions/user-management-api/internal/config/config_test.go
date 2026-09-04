package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/config"
)

// clearEnv puts every variable Load reads into a known-empty state so a test
// never inherits a value from the developer's shell or from a sibling test.
// Load treats an empty value as absent, so this doubles as "unset".
func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"MONGO_URI",
		"MONGO_DATABASE",
		"JWT_SECRET",
		"JWT_TTL",
		"HTTP_PORT",
		"GRPC_PORT",
		"USER_COUNT_LOG_INTERVAL",
	} {
		t.Setenv(key, "")
	}
}

// setRequired supplies only the two variables Load refuses to default.
func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("MONGO_URI", "mongodb://localhost:27017")
	t.Setenv("JWT_SECRET", "test-secret")
}

func TestLoad_Defaults(t *testing.T) {
	clearEnv(t)
	setRequired(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.MongoURI != "mongodb://localhost:27017" {
		t.Errorf("MongoURI = %q, want mongodb://localhost:27017", cfg.MongoURI)
	}
	if cfg.JWTSecret != "test-secret" {
		t.Errorf("JWTSecret = %q, want test-secret", cfg.JWTSecret)
	}
	if cfg.MongoDatabase != "user_management" {
		t.Errorf("MongoDatabase = %q, want user_management", cfg.MongoDatabase)
	}
	if cfg.HTTPPort != "8080" {
		t.Errorf("HTTPPort = %q, want 8080", cfg.HTTPPort)
	}
	if cfg.GRPCPort != "9090" {
		t.Errorf("GRPCPort = %q, want 9090", cfg.GRPCPort)
	}
	if cfg.JWTTTL != 24*time.Hour {
		t.Errorf("JWTTTL = %v, want 24h", cfg.JWTTTL)
	}
	// The challenge fixes this cadence at 10s; the variable only exists so
	// tests and demos can speed it up.
	if cfg.UserCountLogInterval != 10*time.Second {
		t.Errorf("UserCountLogInterval = %v, want 10s", cfg.UserCountLogInterval)
	}
}

func TestLoad_Overrides(t *testing.T) {
	clearEnv(t)
	setRequired(t)
	t.Setenv("MONGO_DATABASE", "other_db")
	t.Setenv("HTTP_PORT", "18080")
	t.Setenv("GRPC_PORT", "19090")
	t.Setenv("JWT_TTL", "30m")
	t.Setenv("USER_COUNT_LOG_INTERVAL", "250ms")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.MongoDatabase != "other_db" {
		t.Errorf("MongoDatabase = %q, want other_db", cfg.MongoDatabase)
	}
	if cfg.HTTPPort != "18080" {
		t.Errorf("HTTPPort = %q, want 18080", cfg.HTTPPort)
	}
	if cfg.GRPCPort != "19090" {
		t.Errorf("GRPCPort = %q, want 19090", cfg.GRPCPort)
	}
	if cfg.JWTTTL != 30*time.Minute {
		t.Errorf("JWTTTL = %v, want 30m", cfg.JWTTTL)
	}
	if cfg.UserCountLogInterval != 250*time.Millisecond {
		t.Errorf("UserCountLogInterval = %v, want 250ms", cfg.UserCountLogInterval)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	tests := map[string]struct {
		mongoURI  string
		jwtSecret string
		wantIn    string
	}{
		"missing MONGO_URI":  {mongoURI: "", jwtSecret: "test-secret", wantIn: "MONGO_URI"},
		"missing JWT_SECRET": {mongoURI: "mongodb://localhost:27017", jwtSecret: "", wantIn: "JWT_SECRET"},
		"missing both":       {mongoURI: "", jwtSecret: "", wantIn: "MONGO_URI"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("MONGO_URI", tt.mongoURI)
			t.Setenv("JWT_SECRET", tt.jwtSecret)

			cfg, err := config.Load()
			if err == nil {
				t.Fatalf("Load() error = nil, want an error naming %s", tt.wantIn)
			}
			if !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Load() error = %q, want it to name %s", err, tt.wantIn)
			}
			if cfg != (config.Config{}) {
				t.Errorf("Load() config = %+v, want the zero Config on error", cfg)
			}
		})
	}
}

func TestLoad_MalformedDuration(t *testing.T) {
	tests := map[string]string{
		"JWT_TTL":                 "not-a-duration",
		"USER_COUNT_LOG_INTERVAL": "10",
	}

	for key, value := range tests {
		t.Run("rejects a malformed "+key, func(t *testing.T) {
			clearEnv(t)
			setRequired(t)
			t.Setenv(key, value)

			cfg, err := config.Load()
			if err == nil {
				t.Fatalf("Load() error = nil, want a parse error for %s=%q", key, value)
			}
			if cfg != (config.Config{}) {
				t.Errorf("Load() config = %+v, want the zero Config on error", cfg)
			}
		})
	}
}
