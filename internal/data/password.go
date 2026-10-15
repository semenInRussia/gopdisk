package data

import (
	"bytes"

	"golang.org/x/crypto/bcrypt"
)

type Password struct {
	hash []byte
}

func (p *Password) Set(plaintext string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	p.hash = hash
	return nil
}

func (p *Password) Matches(plaintext string) (bool, error) {
	if bytes.Equal(p.hash, []byte("!")) {
		return false, nil
	}
	err := bcrypt.CompareHashAndPassword(p.hash, []byte(plaintext))
	if err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (p *Password) Hash() []byte {
	return p.hash
}

func (p *Password) setHash(hash []byte) {
	p.hash = hash
}
