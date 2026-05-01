package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/go-webauthn/webauthn/webauthn"
)

type CredentialStore struct {
	db *sql.DB
}

func NewCredentialStore(db *sql.DB) *CredentialStore {
	return &CredentialStore{db: db}
}

func (s *CredentialStore) Save(ctx context.Context, tx *sql.Tx, userID string, cred *webauthn.Credential) error {
	raw, err := json.Marshal(cred)
	if err != nil {
		return fmt.Errorf("marshal credential: %w", err)
	}
	encodedID := base64.RawURLEncoding.EncodeToString(cred.ID)
	backupEligible := 0
	if cred.Flags.BackupEligible {
		backupEligible = 1
	}
	backupState := 0
	if cred.Flags.BackupState {
		backupState = 1
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO credentials (id, user_id, credential_json, sign_count, backup_eligible, backup_state)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		encodedID, userID, raw, cred.Authenticator.SignCount, backupEligible, backupState,
	)
	return err
}

func (s *CredentialStore) UpdateAfterLogin(ctx context.Context, cred *webauthn.Credential) error {
	raw, err := json.Marshal(cred)
	if err != nil {
		return fmt.Errorf("marshal credential: %w", err)
	}
	encodedID := base64.RawURLEncoding.EncodeToString(cred.ID)
	_, err = s.db.ExecContext(ctx,
		`UPDATE credentials
		 SET sign_count = ?, credential_json = ?, last_used_at = unixepoch()
		 WHERE id = ?`,
		cred.Authenticator.SignCount, raw, encodedID,
	)
	return err
}
