package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/david-loe/volume-mover/internal/model"
	"github.com/david-loe/volume-mover/internal/service"
)

func newServerForTest(t *testing.T) *Server {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "hosts.yaml")
	svc := service.New(cfg, nil)
	server, err := New(svc, cfg, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func TestBasicAuthGuardsRequests(t *testing.T) {
	os.Setenv("VOLUME_MOVER_WEB_USERNAME", "admin")
	os.Setenv("VOLUME_MOVER_WEB_PASSWORD", "secret")
	defer os.Unsetenv("VOLUME_MOVER_WEB_USERNAME")
	defer os.Unsetenv("VOLUME_MOVER_WEB_PASSWORD")

	server := newServerForTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
	res := httptest.NewRecorder()
	server.router.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.Code)
	}
}

func TestHostsAPIUsesFrontendJSONShape(t *testing.T) {
	server := newServerForTest(t)
	payload := []byte(`{"name":"remote","kind":"ssh","host":"remote.example","port":22}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/hosts", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	server.router.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
	res = httptest.NewRecorder()
	server.router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}

	var payloadOut struct {
		Hosts []map[string]any `json:"hosts"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payloadOut); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payloadOut.Hosts) != 2 {
		t.Fatalf("expected local+remote hosts, got %d", len(payloadOut.Hosts))
	}

	var foundRemote map[string]any
	for _, host := range payloadOut.Hosts {
		if host["name"] == "remote" {
			foundRemote = host
			break
		}
	}
	if foundRemote == nil {
		t.Fatalf("expected remote host in response, got %#v", payloadOut.Hosts)
	}
	if _, ok := foundRemote["Name"]; ok {
		t.Fatalf("expected lowercase json fields, got %#v", foundRemote)
	}
	if foundRemote["kind"] != "ssh" {
		t.Fatalf("expected ssh kind, got %#v", foundRemote["kind"])
	}
	if foundRemote["host"] != "remote.example" {
		t.Fatalf("expected host target, got %#v", foundRemote["host"])
	}
}

func TestCreateJobValidationError(t *testing.T) {
	server := newServerForTest(t)
	payload := []byte(`{"operation":"copy","sourceHost":"local","destinationHost":"local","items":[{"sourceVolume":"src","destinationVolume":""}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transfers/jobs", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	server.router.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "required") {
		t.Fatalf("expected validation error body, got %s", res.Body.String())
	}
}

func TestFilterNamedVolumesSkipsAnonymousLookingNames(t *testing.T) {
	volumes := []model.VolumeSummary{
		{Name: "project_data"},
		{Name: "f4f0d1c2b3a497887766554433221100ffeeddccbbaa99887766554433221100"},
		{Name: "db-backup"},
	}

	filtered := filterNamedVolumes(volumes)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 named volumes, got %d", len(filtered))
	}
	if filtered[0].Name != "project_data" || filtered[1].Name != "db-backup" {
		t.Fatalf("unexpected filtered result: %+v", filtered)
	}
}
