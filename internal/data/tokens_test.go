package data

import (
	"crypto/sha256"
	"testing"
	"time"
)

func TestGenerateToken(t *testing.T) {
	userID := GenerateUserID()
	before := time.Now().Add(71 * time.Hour)
	token, err := GenerateToken(userID, 72*time.Hour, ScopeActivation)
	if err != nil {
		t.Fatalf("cannot generate token: %v", err)
	}
	if len(token.Plaintext) != TokenLength {
		t.Fatalf("got plaintext length %d, expected %d", len(token.Plaintext), TokenLength)
	}
	expectedHash := sha256.Sum256([]byte(token.Plaintext))
	if string(token.Hash) != string(expectedHash[:]) {
		t.Fatal("stored hash does not match plaintext token")
	}
	if token.UserID != userID || token.Scope != ScopeActivation {
		t.Fatalf("got token metadata %+v", token)
	}
	if token.Expiry.Before(before) {
		t.Fatalf("token expiry %v is too early", token.Expiry)
	}

	second, err := GenerateToken(userID, time.Hour, ScopeActivation)
	if err != nil {
		t.Fatalf("cannot generate second token: %v", err)
	}
	if second.Plaintext == token.Plaintext {
		t.Fatal("two generated tokens have identical plaintext")
	}
}
