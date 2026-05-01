package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/xuefz/world-fog/internal/config"
)

type WellKnownHandler struct {
	cfg *config.AppleConfig
}

func NewWellKnownHandler(cfg *config.AppleConfig) *WellKnownHandler {
	return &WellKnownHandler{cfg: cfg}
}

func (h *WellKnownHandler) AppleAppSiteAssociation(w http.ResponseWriter, r *http.Request) {
	appID := fmt.Sprintf("%s.%s", h.cfg.TeamID, h.cfg.BundleID)
	payload := map[string]any{
		"webcredentials": map[string]any{
			"apps": []string{appID},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *WellKnownHandler) WebAuthnRelatedOrigins(w http.ResponseWriter, r *http.Request) {
	// Placeholder for WebAuthn Related Origin Requests (ROR) support.
	payload := map[string]any{
		"origins": []string{},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}
