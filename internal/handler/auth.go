package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/xuefz/world-fog/internal/models"
	"github.com/xuefz/world-fog/internal/store"
	"github.com/xuefz/world-fog/internal/token"
)

const sessionTTL = 5 * time.Minute

type AuthHandler struct {
	wa        *webauthn.WebAuthn
	db        *sql.DB
	users     *store.UserStore
	creds     *store.CredentialStore
	sessions  *store.SessionStore
	tokens    *token.Manager
}

func NewAuthHandler(
	wa *webauthn.WebAuthn,
	db *sql.DB,
	users *store.UserStore,
	creds *store.CredentialStore,
	sessions *store.SessionStore,
	tokens *token.Manager,
) *AuthHandler {
	return &AuthHandler{
		wa:       wa,
		db:       db,
		users:    users,
		creds:    creds,
		sessions: sessions,
		tokens:   tokens,
	}
}

// POST /api/v1/auth/register/begin
func (h *AuthHandler) RegisterBegin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DisplayName == "" {
		writeError(w, http.StatusBadRequest, "display_name required")
		return
	}

	user := &models.User{
		ID:          uuid.NewString(),
		DisplayName: req.DisplayName,
	}

	requireResidentKey := true
	options, sessionData, err := h.wa.BeginRegistration(user,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			RequireResidentKey:      &requireResidentKey,
			ResidentKey:             protocol.ResidentKeyRequirementRequired,
			UserVerification:        protocol.VerificationRequired,
			AuthenticatorAttachment: protocol.Platform,
		}),
		webauthn.WithConveyancePreference(protocol.PreferNoAttestation),
	)
	if err != nil {
		slog.Error("begin registration", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to begin registration")
		return
	}

	sessionID := uuid.NewString()
	if err := h.sessions.Save(r.Context(), sessionID, sessionData, sessionTTL); err != nil {
		slog.Error("save session", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to save session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"session_id":   sessionID,
		"user_id":      user.ID,
		"display_name": user.DisplayName,
		"public_key":   options,
	})
}

// POST /api/v1/auth/register/finish
func (h *AuthHandler) RegisterFinish(w http.ResponseWriter, r *http.Request) {
	var envelope struct {
		SessionID   string          `json:"session_id"`
		DisplayName string          `json:"display_name"`
		UserID      string          `json:"user_id"`
		Credential  json.RawMessage `json:"credential"`
	}
	if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	sessionID := envelope.SessionID
	if sessionID == "" {
		sessionID = r.Header.Get("X-Session-ID")
	}
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id required")
		return
	}

	sessionData, err := h.sessions.Get(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusBadRequest, "session expired or not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "session lookup failed")
		return
	}

	user := &models.User{
		ID:          envelope.UserID,
		DisplayName: envelope.DisplayName,
	}

	// Parse the credential from the wrapped field, or from raw body if not wrapped.
	var credBody []byte
	if len(envelope.Credential) > 0 {
		credBody = envelope.Credential
	} else {
		writeError(w, http.StatusBadRequest, "credential required")
		return
	}

	parsedResponse, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(credBody))
	if err != nil {
		slog.Error("parse credential creation response", "err", err)
		writeError(w, http.StatusBadRequest, "invalid credential response")
		return
	}

	credential, err := h.wa.CreateCredential(user, *sessionData, parsedResponse)
	if err != nil {
		slog.Error("create credential", "err", err)
		writeError(w, http.StatusBadRequest, "credential verification failed")
		return
	}

	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(r.Context(),
		`INSERT INTO users (id, display_name) VALUES (?, ?)`,
		user.ID, user.DisplayName,
	); err != nil {
		slog.Error("insert user", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	if err := h.creds.Save(r.Context(), tx, user.ID, credential); err != nil {
		slog.Error("save credential", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to save credential")
		return
	}

	if err := tx.Commit(); err != nil {
		slog.Error("commit", "err", err)
		writeError(w, http.StatusInternalServerError, "db commit failed")
		return
	}

	_ = h.sessions.Delete(r.Context(), sessionID)

	jwtToken, err := h.tokens.Issue(user.ID, user.DisplayName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token":   jwtToken,
		"user_id": user.ID,
	})
}

// POST /api/v1/auth/login/begin
func (h *AuthHandler) LoginBegin(w http.ResponseWriter, r *http.Request) {
	options, sessionData, err := h.wa.BeginDiscoverableLogin()
	if err != nil {
		slog.Error("begin discoverable login", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to begin login")
		return
	}

	sessionID := uuid.NewString()
	if err := h.sessions.Save(r.Context(), sessionID, sessionData, sessionTTL); err != nil {
		slog.Error("save login session", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to save session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": sessionID,
		"public_key": options,
	})
}

// POST /api/v1/auth/login/finish
func (h *AuthHandler) LoginFinish(w http.ResponseWriter, r *http.Request) {
	var envelope struct {
		SessionID  string          `json:"session_id"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	sessionID := envelope.SessionID
	if sessionID == "" {
		sessionID = r.Header.Get("X-Session-ID")
	}
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id required")
		return
	}

	sessionData, err := h.sessions.Get(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusBadRequest, "session expired or not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "session lookup failed")
		return
	}

	if len(envelope.Credential) == 0 {
		writeError(w, http.StatusBadRequest, "credential required")
		return
	}

	parsedResponse, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(envelope.Credential))
	if err != nil {
		slog.Error("parse credential request response", "err", err)
		writeError(w, http.StatusBadRequest, "invalid credential response")
		return
	}

	var foundUser *models.User
	discoverableHandler := func(rawID, userHandle []byte) (webauthn.User, error) {
		u, err := h.users.GetByCredentialID(r.Context(), rawID)
		if err != nil {
			return nil, err
		}
		foundUser = u
		return u, nil
	}

	credential, err := h.wa.ValidateDiscoverableLogin(discoverableHandler, *sessionData, parsedResponse)
	if err != nil {
		slog.Error("validate discoverable login", "err", err)
		writeError(w, http.StatusUnauthorized, "authentication failed")
		return
	}

	if err := h.creds.UpdateAfterLogin(r.Context(), credential); err != nil {
		slog.Error("update credential after login", "err", err)
	}

	_ = h.sessions.Delete(r.Context(), sessionID)

	jwtToken, err := h.tokens.Issue(foundUser.ID, foundUser.DisplayName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token":   jwtToken,
		"user_id": foundUser.ID,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
