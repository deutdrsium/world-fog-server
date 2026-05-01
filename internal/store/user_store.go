package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/xuefz/world-fog/internal/models"
)

var ErrNotFound = errors.New("not found")

type UserStore struct {
	db *sql.DB
}

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) Create(ctx context.Context, u *models.User) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, display_name) VALUES (?, ?)`,
		u.ID, u.DisplayName,
	)
	return err
}

func (s *UserStore) GetByID(ctx context.Context, id string) (*models.User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, display_name FROM users WHERE id = ?`, id)
	u := &models.User{}
	if err := row.Scan(&u.ID, &u.DisplayName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	creds, err := s.loadCredentials(ctx, id)
	if err != nil {
		return nil, err
	}
	u.Credentials = creds
	return u, nil
}

// GetByCredentialID looks up a user by the raw credential ID bytes (used in discoverable login).
func (s *UserStore) GetByCredentialID(ctx context.Context, credID []byte) (*models.User, error) {
	encodedID := base64.RawURLEncoding.EncodeToString(credID)
	row := s.db.QueryRowContext(ctx,
		`SELECT u.id, u.display_name FROM users u
		 JOIN credentials c ON c.user_id = u.id
		 WHERE c.id = ?`, encodedID)
	u := &models.User{}
	if err := row.Scan(&u.ID, &u.DisplayName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	creds, err := s.loadCredentials(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	u.Credentials = creds
	return u, nil
}

func (s *UserStore) loadCredentials(ctx context.Context, userID string) ([]webauthn.Credential, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT credential_json FROM credentials WHERE user_id = ?`, userID)
	if err != nil {
		return nil, fmt.Errorf("query credentials: %w", err)
	}
	defer rows.Close()

	var creds []webauthn.Credential
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var c webauthn.Credential
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, fmt.Errorf("unmarshal credential: %w", err)
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}
