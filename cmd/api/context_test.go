package main

import (
	"context"
	"gopherdisk/internal/data"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUserContext(t *testing.T) {
	app := &Application{}
	type originalKey struct{}
	original := httptest.NewRequest(http.MethodGet, "/", nil)
	original = original.WithContext(context.WithValue(original.Context(), originalKey{}, "preserved"))

	for _, user := range []*data.User{data.AnonymousUser, {ID: "user-id", Name: "Alice"}} {
		r := app.contextSetUser(original, user)
		if got := app.contextGetUser(r); got != user {
			t.Fatalf("got user %v, want %v", got, user)
		}
		if got := r.Context().Value(originalKey{}); got != "preserved" {
			t.Fatalf("original context value lost: %v", got)
		}
		if original.Context().Value(userContextKey) != nil {
			t.Fatal("original request was modified")
		}
	}

	if !data.AnonymousUser.IsAnonymous() || (&data.User{}).IsAnonymous() {
		t.Fatal("only the anonymous user sentinel should be anonymous")
	}
}

func TestContextGetUserPanicsWithoutUser(t *testing.T) {
	app := &Application{}
	defer func() {
		if recover() == nil {
			t.Error("expected panic for missing user")
		}
	}()
	app.contextGetUser(httptest.NewRequest(http.MethodGet, "/", nil))
}
