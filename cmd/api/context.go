package main

import (
	"context"
	"gopherdisk/internal/data"
	"net/http"
)

type contextKey string

const userContextKey contextKey = "user"

func (app *Application) contextSetUser(r *http.Request, user *data.User) *http.Request {
	ctx := context.WithValue(r.Context(), userContextKey, user)
	return r.WithContext(ctx)
}

func (app *Application) contextGetUser(r *http.Request) *data.User {
	user, ok := r.Context().Value(userContextKey).(*data.User)
	if !ok || user == nil {
		panic("missing user value in request context")
	}
	return user
}
