package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/irgordon/kari/api/internal/core/domain"
)

type errorResponse struct {
	Message string `json:"message"`
}

func HandleError(w http.ResponseWriter, _ *http.Request, err error) {
	status := statusForError(err)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Message: err.Error()})
}

func statusForError(err error) int {
	switch {
	case errors.Is(err, domain.ErrInvalidCredentials):
		return http.StatusUnauthorized
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}
