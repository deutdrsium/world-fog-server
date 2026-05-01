package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	WebAuthn WebAuthnConfig `yaml:"webauthn"`
	JWT      JWTConfig      `yaml:"jwt"`
	DB       DBConfig       `yaml:"db"`
	Apple    AppleConfig    `yaml:"apple"`
}

type ServerConfig struct {
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	TLSCert string `yaml:"tls_cert"`
	TLSKey  string `yaml:"tls_key"`
}

type WebAuthnConfig struct {
	RPID          string   `yaml:"rp_id"`
	RPDisplayName string   `yaml:"rp_display_name"`
	RPOrigins     []string `yaml:"rp_origins"`
}

type JWTConfig struct {
	Secret    string `yaml:"secret"`
	ExpiryHrs int    `yaml:"expiry_hrs"`
}

type DBConfig struct {
	Path string `yaml:"path"`
}

type AppleConfig struct {
	TeamID   string `yaml:"team_id"`
	BundleID string `yaml:"bundle_id"`
}

func Load(path string) (*Config, error) {
	cfg := defaults()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read config: %w", err)
		}
		if err == nil {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("parse config: %w", err)
			}
		}
	}

	applyEnv(cfg)
	return cfg, nil
}

func defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8443,
		},
		WebAuthn: WebAuthnConfig{
			RPID:          "xuefz.cn",
			RPDisplayName: "世界迷雾",
			RPOrigins:     []string{"https://xuefz.cn", "https://api.xuefz.cn"},
		},
		JWT: JWTConfig{
			ExpiryHrs: 720,
		},
		DB: DBConfig{
			Path: "./world-fog.db",
		},
	}
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("WF_SERVER_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("WF_SERVER_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}
	if v := os.Getenv("WF_SERVER_TLS_CERT"); v != "" {
		cfg.Server.TLSCert = v
	}
	if v := os.Getenv("WF_SERVER_TLS_KEY"); v != "" {
		cfg.Server.TLSKey = v
	}
	if v := os.Getenv("WF_WEBAUTHN_RP_ID"); v != "" {
		cfg.WebAuthn.RPID = v
	}
	if v := os.Getenv("WF_WEBAUTHN_RP_DISPLAY_NAME"); v != "" {
		cfg.WebAuthn.RPDisplayName = v
	}
	if v := os.Getenv("WF_WEBAUTHN_RP_ORIGINS"); v != "" {
		cfg.WebAuthn.RPOrigins = strings.Split(v, ",")
	}
	if v := os.Getenv("WF_JWT_SECRET"); v != "" {
		cfg.JWT.Secret = v
	}
	if v := os.Getenv("WF_JWT_EXPIRY_HRS"); v != "" {
		if h, err := strconv.Atoi(v); err == nil {
			cfg.JWT.ExpiryHrs = h
		}
	}
	if v := os.Getenv("WF_DB_PATH"); v != "" {
		cfg.DB.Path = v
	}
	if v := os.Getenv("WF_APPLE_TEAM_ID"); v != "" {
		cfg.Apple.TeamID = v
	}
	if v := os.Getenv("WF_APPLE_BUNDLE_ID"); v != "" {
		cfg.Apple.BundleID = v
	}
}
