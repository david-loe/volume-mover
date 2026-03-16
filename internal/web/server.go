package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/david-loe/volume-mover/internal/jobs"
	"github.com/david-loe/volume-mover/internal/model"
	"github.com/david-loe/volume-mover/internal/service"
	"github.com/go-chi/chi/v5"
)

type Server struct {
	service *service.Service
	jobs    *jobs.Manager
	listen  string
	router  http.Handler
}

var anonymousVolumePattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type hostPayload struct {
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Alias        string `json:"alias"`
	Host         string `json:"host"`
	User         string `json:"user"`
	Port         int    `json:"port"`
	IdentityFile string `json:"identityFile"`
}

func New(app *service.Service, configPath string, listen string) (*Server, error) {
	store, err := jobs.NewStore(jobs.DefaultDBPath(configPath))
	if err != nil {
		return nil, err
	}
	manager, err := jobs.NewManager(store, app)
	if err != nil {
		return nil, err
	}
	s := &Server{service: app, jobs: manager, listen: listen}
	r := chi.NewRouter()
	r.Use(s.basicAuth)
	r.Get("/", s.redirectDashboard)
	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/hosts", s.apiHosts)
		api.Post("/hosts/import-ssh", s.apiImportSSH)
		api.Post("/hosts", s.apiSaveHost)
		api.Delete("/hosts/{name}", s.apiDeleteHost)
		api.Post("/hosts/{name}/test", s.apiTestHost)
		api.Get("/volumes", s.apiVolumes)
		api.Get("/volumes/{host}/{name}", s.apiVolumeDetail)
		api.Post("/transfers/jobs", s.apiCreateJob)
		api.Get("/transfers/jobs", s.apiJobs)
		api.Get("/transfers/jobs/{id}", s.apiJob)
		api.Get("/transfers/jobs/{id}/events", s.apiJobEvents)
		api.Post("/transfers/jobs/{id}/cancel", s.apiCancelJob)
	})
	r.Get("/app", s.redirectDashboard)
	r.Get("/app/*", s.serveSPA)
	r.Get("/hosts", s.redirectLegacy("/app/hosts"))
	r.Get("/volumes", s.redirectLegacy("/app/volumes"))
	r.Get("/volumes/{host}/{name}", s.redirectVolumeDetail)
	r.Get("/transfer", s.redirectLegacy("/app/transfers/new"))
	r.Post("/hosts/import-ssh", s.redirectDashboard)
	r.Post("/hosts/save", s.redirectDashboard)
	r.Post("/hosts/delete", s.redirectDashboard)
	r.Post("/hosts/test", s.redirectDashboard)
	r.Post("/transfer", s.redirectDashboard)
	r.Handle("/assets/*", spaAssets())
	s.router = r
	return s, nil
}

func (s *Server) Run() error {
	return http.ListenAndServe(s.listen, s.router)
}

func (s *Server) redirectDashboard(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/app/dashboard", http.StatusSeeOther)
}

func (s *Server) redirectLegacy(target string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		redirectWithRawQuery(w, r, target)
	}
}

func (s *Server) redirectVolumeDetail(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	name := chi.URLParam(r, "name")
	target := "/app/volumes/" + url.PathEscape(host) + "/" + url.PathEscape(name)
	redirectWithRawQuery(w, r, target)
}

func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	serveSPAIndex(w, r)
}

func (s *Server) apiHosts(w http.ResponseWriter, r *http.Request) {
	hosts, err := s.service.ListHosts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hosts": hosts})
}

func (s *Server) apiImportSSH(w http.ResponseWriter, r *http.Request) {
	hosts, err := s.service.ImportSSHHosts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hosts": hosts})
}

func (s *Server) apiSaveHost(w http.ResponseWriter, r *http.Request) {
	var payload hostPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	host := model.HostConfig{
		Name:         payload.Name,
		Kind:         model.HostKind(payload.Kind),
		Alias:        payload.Alias,
		Host:         payload.Host,
		User:         payload.User,
		Port:         payload.Port,
		IdentityFile: payload.IdentityFile,
	}
	if err := s.service.AddHost(host); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"host": host})
}

func (s *Server) apiDeleteHost(w http.ResponseWriter, r *http.Request) {
	if err := s.service.DeleteHost(chi.URLParam(r, "name")); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) apiTestHost(w http.ResponseWriter, r *http.Request) {
	version, err := s.service.TestHost(r.Context(), chi.URLParam(r, "name"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": version})
}

func (s *Server) apiVolumes(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	if host == "" {
		host = "local"
	}
	hideAnonymous := r.URL.Query().Get("hideAnonymous") != "0"
	volumes, err := s.service.ListVolumes(r.Context(), host)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if hideAnonymous {
		volumes = filterNamedVolumes(volumes)
	}
	if volumes == nil {
		volumes = []model.VolumeSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"host": host, "volumes": volumes})
}

func (s *Server) apiVolumeDetail(w http.ResponseWriter, r *http.Request) {
	detail, err := s.service.VolumeDetail(r.Context(), chi.URLParam(r, "host"), chi.URLParam(r, "name"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) apiCreateJob(w http.ResponseWriter, r *http.Request) {
	var req model.CreateTransferJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	job, err := s.jobs.CreateJob(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, model.CreateTransferJobResponse{JobID: job.ID})
}

func (s *Server) apiJobs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	jobsList, err := s.jobs.ListJobs(r.Context(), jobs.ListFilter{
		Host:      r.URL.Query().Get("host"),
		Operation: r.URL.Query().Get("operation"),
		Status:    r.URL.Query().Get("status"),
		Limit:     limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if jobsList == nil {
		jobsList = []model.TransferJob{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobsList})
}

func (s *Server) apiJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.jobs.Job(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, context.Canceled) {
			status = http.StatusRequestTimeout
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) apiJobEvents(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	existing, err := s.jobs.Events(r.Context(), jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, event := range existing {
		streamEvent(w, event)
	}
	flusher.Flush()
	ch, cancel := s.jobs.Subscribe(jobID)
	defer cancel()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-ch:
			streamEvent(w, event)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) apiCancelJob(w http.ResponseWriter, r *http.Request) {
	if err := s.jobs.Cancel(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "cancelling"})
}

func (s *Server) basicAuth(next http.Handler) http.Handler {
	username := os.Getenv("VOLUME_MOVER_WEB_USERNAME")
	password := os.Getenv("VOLUME_MOVER_WEB_PASSWORD")
	if username == "" || password == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != username || pass != password {
			w.Header().Set("WWW-Authenticate", `Basic realm="volume-mover"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

func streamEvent(w http.ResponseWriter, event model.TransferJobEvent) {
	data, _ := json.Marshal(event)
	fmt.Fprintf(w, "id: %d\n", event.ID)
	fmt.Fprintf(w, "data: %s\n\n", data)
}

func redirectWithRawQuery(w http.ResponseWriter, r *http.Request, path string) {
	target := path
	if raw := r.URL.RawQuery; raw != "" {
		target += "?" + raw
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func filterNamedVolumes(volumes []model.VolumeSummary) []model.VolumeSummary {
	filtered := make([]model.VolumeSummary, 0, len(volumes))
	for _, volume := range volumes {
		if anonymousVolumePattern.MatchString(volume.Name) {
			continue
		}
		filtered = append(filtered, volume)
	}
	return filtered
}
