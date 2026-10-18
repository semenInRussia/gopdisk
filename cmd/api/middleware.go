package main

import (
	"errors"
	"fmt"
	"gopherdisk/internal/data"
	"gopherdisk/internal/validator"
	"net/http"
	"strings"
)

func (app *Application) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Authorization")

		authorizationHeader := r.Header.Get("Authorization")
		if authorizationHeader == "" {
			r = app.contextSetUser(r, data.AnonymousUser)
			next.ServeHTTP(w, r)
			return
		}

		headerParts := strings.Split(authorizationHeader, " ")
		if len(headerParts) != 2 || headerParts[0] != "Bearer" {
			app.invalidAuthenticationTokenResponse(w)
			return
		}

		token := headerParts[1]
		v := validator.New()
		data.ValidateTokenPlaintext(v, token)
		if !v.IsValid() {
			app.invalidAuthenticationTokenResponse(w)
			return
		}

		user, err := app.storage.Users.GetForToken(r.Context(), data.ScopeAuthentication, token)
		if err != nil {
			switch {
			case errors.Is(err, data.ErrInvalidToken):
				app.invalidAuthenticationTokenResponse(w)
			default:
				app.serverErrorResponse(w, err)
			}
			return
		}

		r = app.contextSetUser(r, user)
		next.ServeHTTP(w, r)
	})
}

func (app *Application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				w.Header().Set("Connection", "close")
				app.errorResponse(w, fmt.Sprintf("%v", err), http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}
