package main

import (
	"context"
	"errors"
	"fmt"
	"gopherdisk/internal/data"
	"gopherdisk/internal/validator"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// A 16 MiB part can take minutes over a mobile connection. Extend both socket
// deadlines: WriteTimeout also starts before the request body has been read.
func (app *Application) uploadPartTimeout(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const timeout = 10 * time.Minute
		deadline := time.Now().Add(timeout)
		controller := http.NewResponseController(w)
		for _, err := range []error{controller.SetReadDeadline(deadline), controller.SetWriteDeadline(deadline)} {
			if err != nil && !errors.Is(err, http.ErrNotSupported) {
				app.serverErrorResponse(w, err)
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next(w, r.WithContext(ctx))
	}
}

func (app *Application) enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		w.Header().Add("Vary", "Access-Control-Request-Method")

		origin := r.Header.Get("Origin")
		if origin != "" && slices.Contains(app.cfg.cors.trustedOrigins, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, PUT, PATCH, DELETE")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.WriteHeader(http.StatusOK)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

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

func (app *Application) requireAuthenticatedUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if app.contextGetUser(r).IsAnonymous() {
			app.authenticationRequiredResponse(w)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func (app *Application) requireActivatedUser(next http.HandlerFunc) http.HandlerFunc {
	return app.requireAuthenticatedUser(func(w http.ResponseWriter, r *http.Request) {
		if !app.contextGetUser(r).Activated {
			app.inactiveAccountResponse(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (app *Application) requireCurrentUser(next http.HandlerFunc) http.HandlerFunc {
	return app.requireActivatedUser(func(w http.ResponseWriter, r *http.Request) {
		id, err := app.getUUIDParam(r, "id")
		if err != nil {
			app.badRequestErrorResponse(w, err.Error())
			return
		}
		if uuid.MustParse(id).String() != string(app.contextGetUser(r).ID) {
			app.notPermittedResponse(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (app *Application) requireFilePermission(param, code string, next http.HandlerFunc) http.HandlerFunc {
	return app.requireActivatedUser(func(w http.ResponseWriter, r *http.Request) {
		id, err := app.getUUIDParam(r, param)
		if err != nil {
			app.badRequestErrorResponse(w, err.Error())
			return
		}
		allowed, err := app.storage.Permissions.HasForFile(r.Context(), data.FileID(id), app.contextGetUser(r).ID, code)
		if err != nil {
			switch {
			case errors.Is(err, data.ErrFileNotFound):
				app.notFoundResponse(w, r)
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				return
			default:
				app.serverErrorResponse(w, err)
			}
			return
		}
		if !allowed {
			app.notPermittedResponse(w)
			return
		}
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
