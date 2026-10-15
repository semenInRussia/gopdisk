package data

import (
	"bytes"
	"testing"
)

func TestPasswordSetAndMatches(t *testing.T) {
	const plaintext = "correct horse battery staple"
	var password Password
	if err := password.Set(plaintext); err != nil {
		t.Fatalf("cannot hash password: %v", err)
	}
	if bytes.Equal(password.Hash(), []byte(plaintext)) {
		t.Fatal("password hash contains plaintext")
	}
	matches, err := password.Matches(plaintext)
	if err != nil || !matches {
		t.Fatalf("correct password did not match: matches=%v err=%v", matches, err)
	}
	matches, err = password.Matches("wrong password")
	if err != nil || matches {
		t.Fatalf("wrong password result: matches=%v err=%v", matches, err)
	}
}

func TestLegacyPasswordNeverMatches(t *testing.T) {
	password := Password{hash: []byte("!")}
	matches, err := password.Matches("anything")
	if err != nil || matches {
		t.Fatalf("legacy password result: matches=%v err=%v", matches, err)
	}
}
