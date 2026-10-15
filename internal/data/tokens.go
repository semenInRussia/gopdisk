package data

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"time"

	"gopherdisk/internal/validator"
)

const (
	ScopeActivation  = "activation"
	tokenRandomBytes = 16
	TokenLength      = 26
)

type Token struct {
	Plaintext string    `json:"token"`
	Hash      []byte    `json:"-"`
	UserID    UserID    `json:"-"`
	Expiry    time.Time `json:"expiry"`
	Scope     string    `json:"-"`
}

type TokenModel struct {
	db *sql.DB
}

func GenerateToken(userID UserID, ttl time.Duration, scope string) (*Token, error) {
	randomBytes := make([]byte, tokenRandomBytes)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, err
	}

	plaintext := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(randomBytes)
	hash := sha256.Sum256([]byte(plaintext))

	return &Token{
		Plaintext: plaintext,
		Hash:      hash[:],
		UserID:    userID,
		Expiry:    time.Now().Add(ttl),
		Scope:     scope,
	}, nil
}

func (m TokenModel) New(ctx context.Context, userID UserID, ttl time.Duration, scope string) (*Token, error) {
	token, err := GenerateToken(userID, ttl, scope)
	if err != nil {
		return nil, err
	}
	if err := insertToken(ctx, m.db, *token); err != nil {
		return nil, err
	}
	return token, nil
}

func (m TokenModel) DeleteAllForUser(ctx context.Context, scope string, userID UserID) error {
	_, err := m.db.ExecContext(ctx, `DELETE FROM tokens WHERE scope = $1 AND user_id = $2`, scope, userID)
	return err
}

func insertToken(ctx context.Context, db dbExecutor, token Token) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO tokens (hash, user_id, expiry, scope)
		VALUES ($1, $2, $3, $4)`, token.Hash, token.UserID, token.Expiry, token.Scope)
	return err
}

func ValidateTokenPlaintext(v *validator.Validator, plaintext string) {
	v.Check("token", plaintext != "", "must be provided")
	v.Check("token", len(plaintext) == TokenLength, "must be 26 bytes long")
}
