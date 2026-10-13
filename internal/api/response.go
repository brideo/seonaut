package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/go-sql-driver/mysql"
	"github.com/stjudewashere/seonaut/internal/services"
)

const contentTypeJSON = "application/json"

// Error codes returned in the "code" field of the error envelope.
const (
	codeBadRequest       = "bad_request"
	codeValidation       = "validation_error"
	codeUnauthorized     = "unauthorized"
	codeNotFound         = "not_found"
	codeMethodNotAllowed = "method_not_allowed"
	codeUserExists       = "user_exists"
	codeCrawlInProgress  = "crawl_in_progress"
	codeProjectDeleting  = "project_deleting"
	codeInternal         = "internal_error"
)

// mysqlForeignKeyError is MySQL's error number for a failed foreign key check.
const mysqlForeignKeyError = 1452

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

// writeJSON encodes v as the response body with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: write response: %v", err)
	}
}

// writeError writes the standard error envelope.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: message}})
}

// writeServiceError maps errors returned by the services to HTTP responses.
// Unknown errors are logged and reported as a generic 500.
func writeServiceError(w http.ResponseWriter, err error) {
	var mysqlErr *mysql.MySQLError

	switch {
	case errors.Is(err, services.ErrInvalidEmail),
		errors.Is(err, services.ErrInvalidPassword),
		errors.Is(err, services.ErrProtocolNotSupported),
		errors.Is(err, services.ErrUserAgent):
		writeError(w, http.StatusBadRequest, codeValidation, err.Error())
	case errors.Is(err, services.ErrUserExists):
		writeError(w, http.StatusConflict, codeUserExists, "user already exists")
	case errors.Is(err, services.ErrCrawlerAlreadyRunning):
		writeError(w, http.StatusConflict, codeCrawlInProgress, "project is already being crawled")
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, http.StatusNotFound, codeNotFound, "project not found")
	case errors.As(err, &mysqlErr) && mysqlErr.Number == mysqlForeignKeyError:
		writeError(w, http.StatusBadRequest, codeValidation, "user does not exist")
	default:
		log.Printf("api: internal error: %v", err)
		writeError(w, http.StatusInternalServerError, codeInternal, "internal server error")
	}
}

// decodeJSON reads a JSON request body into v, rejecting unknown fields and
// oversized bodies. An empty body is allowed when allowEmpty is true.
func decodeJSON(w http.ResponseWriter, r *http.Request, v interface{}, allowEmpty bool) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	err := dec.Decode(v)
	if err == io.EOF && allowEmpty {
		return nil
	}

	return err
}
