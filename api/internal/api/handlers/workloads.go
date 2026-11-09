package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/irgordon/kari/api/internal/api/middleware"
	"github.com/irgordon/kari/api/internal/core/domain"
	"github.com/irgordon/kari/api/internal/core/services"
	pb "github.com/irgordon/kari/api/internal/grpc/rustagent"
)

type WorkloadHandler struct {
	Service *services.Sites
	Auth    *middleware.AuthMiddleware
}

func (h *WorkloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(domain.UserContextKey).(*domain.UserClaims)
	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "site"))
	if err != nil {
		http.Error(w, `{"message":"Invalid site UUID"}`, 400)
		return
	}
	resource := chi.URLParam(r, "resource")
	var input services.WorkloadInput
	if r.Method == "GET" {
		input.Action = r.URL.Query().Get("action")
		input.Path = r.URL.Query().Get("path")
		if input.Action == "" {
			input.Action = "status"
			if resource == "files" || resource == "backups" {
				input.Action = "list"
			}
		}
		if input.Action != "status" && input.Action != "list" && input.Action != "read" && input.Action != "logs" {
			http.Error(w, `{"message":"Mutations require POST"}`, 405)
			return
		}
	} else if json.NewDecoder(r.Body).Decode(&input) != nil {
		http.Error(w, `{"message":"Invalid workload request"}`, 400)
		return
	}
	execute := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := h.Service.Workload(r.Context(), claims.Subject, id, resource, input)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			status, message := 500, "Operation could not be confirmed. Refresh status and inspect host logs before retrying. A restore may require operator repair before resuming the site/SFTP."
			switch {
			case errors.Is(err, services.ErrSiteInput):
				status, message = 400, err.Error()
			case errors.Is(err, services.ErrSiteBusy):
				status, message = 409, "Wait for the current site operation to finish"
			case errors.Is(err, domain.ErrNotFound):
				status, message = 404, "Website not found"
			}
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(map[string]string{"message": message})
			return
		}
		json.NewEncoder(w).Encode(result)
	})
	if input.Action == "delete" || input.Action == "restore" {
		h.Auth.RequirePermission("applications", "delete")(execute).ServeHTTP(w, r)
	} else {
		execute.ServeHTTP(w, r)
	}
}

func (h *WorkloadHandler) System(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var result any
	var err error
	if r.URL.Path == "/api/v1/system/stats" {
		result, err = h.Service.Agent.GetSystemStatus(r.Context(), &pb.Empty{})
	} else {
		input := &pb.HostServiceRequest{Action: "status"}
		if r.Method == "POST" {
			if json.NewDecoder(r.Body).Decode(input) != nil || input.Action != "reload" || input.Service != "nginx" {
				http.Error(w, `{"message":"Only validated Nginx reload is supported"}`, 400)
				return
			}
		}
		result, err = h.Service.Agent.HostServices(r.Context(), input)
	}
	if err != nil {
		w.WriteHeader(503)
		json.NewEncoder(w).Encode(map[string]string{"message": "Native host operation unavailable; inspect the agent and host logs"})
		return
	}
	json.NewEncoder(w).Encode(result)
}
