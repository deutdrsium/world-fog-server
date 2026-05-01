package webauthnutil

import (
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/xuefz/world-fog/internal/config"
)

func New(cfg *config.WebAuthnConfig) (*webauthn.WebAuthn, error) {
	return webauthn.New(&webauthn.Config{
		RPDisplayName: cfg.RPDisplayName,
		RPID:          cfg.RPID,
		RPOrigins:     cfg.RPOrigins,
	})
}
