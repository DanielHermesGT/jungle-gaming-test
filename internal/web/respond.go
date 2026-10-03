package web

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/usecase"
)

type errorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("web: encode response", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: message, Code: code})
}

func mapUseCaseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_input", err.Error())
	case errors.Is(err, usecase.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, usecase.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "resource conflict")
	default:
		slog.Error("web: request failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
