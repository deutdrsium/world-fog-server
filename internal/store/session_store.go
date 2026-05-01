package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

type SessionStore struct {
	db *sql.DB
}

func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

func (s *SessionStore) Save(ctx context.Context, sessionID string, data *webauthn.SessionData, ttl time.Duration) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	expiresAt := time.Now().Add(ttl).Unix()
	_, err = s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO webauthn_sessions (id, session_data, expires_at) VALUES (?, ?, ?)`,
		sessionID, raw, expiresAt,
	)
	return err
}

func (s *SessionStore) Get(ctx context.Context, sessionID string) (*webauthn.SessionData, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT session_data, expires_at FROM webauthn_sessions WHERE id = ?`, sessionID)
	var raw []byte
	var expiresAt int64
	if err := row.Scan(&raw, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if time.Now().Unix() > expiresAt {
		_ = s.Delete(ctx, sessionID)
		return nil, ErrNotFound
	}
	var data webauthn.SessionData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	return &data, nil
}

func (s *SessionStore) Delete(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM webauthn_sessions WHERE id = ?`, sessionID)
	return err
}

// StartCleanup removes expired sessions on the given interval.
func (s *SessionStore) StartCleanup(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = s.db.ExecContext(ctx,
					`DELETE FROM webauthn_sessions WHERE expires_at < unixepoch()`)
			}
		}
	}()
}
