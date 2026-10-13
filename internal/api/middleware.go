package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// requireAPIKey rejects requests that don't carry the configured key either in
// the X-API-Key header or as an Authorization Bearer token.
func requireAPIKey(key string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("X-API-Key")
		if got == "" {
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				got = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
			}
		}

		if key == "" || got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="seonaut"`)
			writeError(w, http.StatusUnauthorized, codeUnauthorized, "invalid or missing API key")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// jsonErrors converts the plain text 404/405 responses that http.ServeMux
// produces into the API's JSON error envelope.
func jsonErrors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&errorRewriter{ResponseWriter: w}, r)
	})
}

type errorRewriter struct {
	http.ResponseWriter
	handled bool
}

func (e *errorRewriter) WriteHeader(status int) {
	if e.ResponseWriter.Header().Get("Content-Type") != contentTypeJSON {
		switch status {
		case http.StatusNotFound:
			e.handled = true
			writeError(e.ResponseWriter, status, codeNotFound, "resource not found")
			return
		case http.StatusMethodNotAllowed:
			e.handled = true
			writeError(e.ResponseWriter, status, codeMethodNotAllowed, "method not allowed")
			return
		}
	}

	e.ResponseWriter.WriteHeader(status)
}

func (e *errorRewriter) Write(b []byte) (int, error) {
	if e.handled {
		return len(b), nil
	}

	return e.ResponseWriter.Write(b)
}
