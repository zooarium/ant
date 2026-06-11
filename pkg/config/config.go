package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Environment string `mapstructure:"ENVIRONMENT"`
	Server      ServerConfig
	Database    DatabaseConfig
	Log         LogConfig  `mapstructure:"LOG"`
	Auth        AuthConfig `mapstructure:"AUTH"`
	CORS        CORSConfig
	Secondary   []SecondaryConfig `mapstructure:"SECONDARY"`
}

// SecondaryConfig drives one optional secondary listener: an additional HTTP
// server in the same process exposing only the allow-listed routes, with
// rate limiting configured independently of the primary server. Any number
// of listeners can be declared under SECONDARY. Identity always comes from
// JWT; JWT_SECRET (optional) makes the listener verify with a different
// signing key (e.g. keeper's guest secret) instead of AUTH.JWT_SECRET.
type SecondaryConfig struct {
	Name      string          `mapstructure:"NAME"`
	Enabled   bool            `mapstructure:"ENABLED"`
	Addr      string          `mapstructure:"ADDR"`
	JWTSecret string          `mapstructure:"JWT_SECRET"`
	RateLimit RateLimitConfig `mapstructure:"RATE_LIMIT"`
	Routes    []string        `mapstructure:"ROUTES"`
}

type RateLimitConfig struct {
	Requests int           `mapstructure:"REQUESTS"`
	Window   time.Duration `mapstructure:"WINDOW"`
}

type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"ALLOWED_ORIGINS"`
}

type ServerConfig struct {
	Addr         string        `mapstructure:"ADDR"`
	Host         string        `mapstructure:"HOST"`
	ReadTimeout  time.Duration `mapstructure:"READ_TIMEOUT"`
	WriteTimeout time.Duration `mapstructure:"WRITE_TIMEOUT"`
	IdleTimeout  time.Duration `mapstructure:"IDLE_TIMEOUT"`
}

type AuthConfig struct {
	JWTSecret string        `mapstructure:"JWT_SECRET"`
	JWTExpiry time.Duration `mapstructure:"JWT_EXPIRY"`
}

type DatabaseConfig struct {
	Path   string `mapstructure:"PATH"`
	Driver string `mapstructure:"DRIVER"`
	DSN    string `mapstructure:"DSN"`
}

type LogConfig struct {
	Dir   string `mapstructure:"DIR"`
	Level string `mapstructure:"LEVEL"`
}

func Load() (*Config, error) {
	v := viper.New()

	v.SetDefault("ENVIRONMENT", "production")
	v.SetDefault("SERVER.ADDR", ":8082")
	v.SetDefault("SERVER.HOST", "localhost:8082")
	v.SetDefault("SERVER.READ_TIMEOUT", 5*time.Second)
	v.SetDefault("SERVER.WRITE_TIMEOUT", 10*time.Second)
	v.SetDefault("SERVER.IDLE_TIMEOUT", 120*time.Second)
	v.SetDefault("DATABASE.PATH", "data/ant.db")
	v.SetDefault("DATABASE.DRIVER", "sqlite3")
	v.SetDefault("DATABASE.DSN", "")
	v.SetDefault("LOG.DIR", "log")
	v.SetDefault("LOG.LEVEL", "info")
	v.SetDefault("AUTH.JWT_SECRET", "a-very-secure-and-shared-secret-key")
	v.SetDefault("AUTH.JWT_EXPIRY", 24*time.Hour)
	v.SetDefault("CORS.ALLOWED_ORIGINS", []string{"*"})

	v.SetEnvPrefix("ANT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")

	v.SetConfigName("config")
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read base config file: %w", err)
		}
	}

	env := v.GetString("ENVIRONMENT")
	if env != "" {
		v.SetConfigName(fmt.Sprintf("config.%s", env))
		if err := v.MergeInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return nil, fmt.Errorf("failed to merge environment config: %w", err)
			}
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := normalizeSecondary(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// normalizeSecondary validates the secondary listener entries and applies
// per-entry defaults (viper defaults cannot reach into list elements).
func normalizeSecondary(cfg *Config) error {
	seen := map[string]bool{cfg.Server.Addr: true}
	for i := range cfg.Secondary {
		s := &cfg.Secondary[i]
		if !s.Enabled {
			continue
		}
		if s.Name == "" {
			s.Name = fmt.Sprintf("secondary-%d", i)
		}
		if s.Addr == "" {
			return fmt.Errorf("SECONDARY[%d] (%s): ADDR is required", i, s.Name)
		}
		if seen[s.Addr] {
			return fmt.Errorf("SECONDARY[%d] (%s): ADDR %q already in use by another listener", i, s.Name, s.Addr)
		}
		seen[s.Addr] = true
		if s.RateLimit.Requests <= 0 {
			s.RateLimit.Requests = 100
		}
		if s.RateLimit.Window <= 0 {
			s.RateLimit.Window = 1 * time.Minute
		}
	}
	return nil
}
