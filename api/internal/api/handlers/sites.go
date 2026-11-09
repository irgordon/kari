package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/irgordon/kari/api/internal/core/domain"
	"github.com/irgordon/kari/api/internal/core/services"
	"github.com/irgordon/kari/api/internal/grpc/rustagent"
)

type SiteHandler struct{ Service *services.Sites }

func (h *SiteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(domain.UserContextKey).(*domain.UserClaims)
	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	var result any
	var err error
	id := uuid.Nil
	if raw := chi.URLParam(r, "site"); raw != "" {
		id, err = uuid.Parse(raw)
		if err != nil {
			http.Error(w, `{"message":"Invalid site ID"}`, 400)
			return
		}
	}
	status := 200
	switch {
	case r.Method == "GET" && id == uuid.Nil:
		result, err = h.Service.List(r.Context(), claims.Subject)
	case r.Method == "GET" && chi.URLParam(r, "deployment") == "":
		result, err = h.Service.Get(r.Context(), claims.Subject, id)
	case r.Method == "POST" && id == uuid.Nil:
		var input struct {
			Domain  string `json:"domain_name"`
			RepoURL string `json:"repo_url"`
			Branch  string `json:"branch"`
			Runtime string `json:"runtime"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			http.Error(w, `{"message":"Invalid site input"}`, 400)
			return
		}
		var site *services.Site
		site, err = h.Service.Create(r.Context(), claims.Subject, input.Domain, input.RepoURL, input.Branch, input.Runtime)
		result = site
		if err == nil {
			_, err = h.Service.Agent.SiteRuntime(r.Context(), &rustagent.RuntimeRequest{SiteId: site.ID.String(), DomainName: site.Domain, Action: "initialize"})
		}
		status = 201
	case chi.URLParam(r, "deployment") != "":
		deployment, parseErr := uuid.Parse(chi.URLParam(r, "deployment"))
		if parseErr != nil {
			http.Error(w, `{"message":"Invalid deployment ID"}`, 400)
			return
		}
		result, err = h.Service.Deployment(r.Context(), claims.Subject, id, deployment)
	case chi.URLParam(r, "operation") == "deploy":
		var trace string
		trace, err = h.Service.Deploy(r.Context(), claims.Subject, id)
		result = map[string]string{"id": trace}
		status = 202
	case chi.URLParam(r, "operation") == "tls":
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
		defer cancel()
		var input struct {
			Chain string `json:"certificate_pem"`
			Key   string `json:"private_key_pem"`
			Agree bool   `json:"agree_to_terms"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			http.Error(w, `{"message":"Invalid TLS request"}`, 400)
			return
		}
		if input.Chain != "" || input.Key != "" {
			err = h.Service.ImportTLS(ctx, claims.Subject, id, []byte(input.Chain), []byte(input.Key))
			input.Key = ""
		} else {
			if !input.Agree {
				http.Error(w, `{"message":"Agree to the ACME subscriber agreement before requesting a certificate"}`, 400)
				return
			}
			err = h.Service.IssueTLS(ctx, claims.Subject, id)
		}
		result = map[string]string{"message": "Certificate installed and Nginx reloaded"}
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		code, message := 500, "Site operation failed. Check service logs and try again."
		switch {
		case errors.Is(err, services.ErrSiteInput):
			code, message = 400, err.Error()
		case errors.Is(err, domain.ErrNotFound):
			code, message = 404, "Site or deployment not found"
		case errors.Is(err, services.ErrSiteBusy):
			code, message = 409, "Publish the site first and wait for its current operation to finish"
		}
		if id != uuid.Nil && chi.URLParam(r, "operation") == "tls" && code == 500 {
			message = "TLS completion could not be confirmed. Check DNS, inbound ports 80/443, certificate details, and the host state before retrying."
		}
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"message": message})
		return
	}
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(result)
}
