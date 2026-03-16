package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/david-loe/volume-mover/internal/model"
)

func TestImportSSHHosts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	content := `Host prod
  HostName 10.0.0.5
  User root
  Port 2222
  IdentityFile ~/.ssh/id_prod

Host *.wild
  User ignored

Host backup backup-alt
  HostName backup.internal
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	hosts, err := ImportSSHHosts(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 3 {
		t.Fatalf("expected 3 hosts, got %d", len(hosts))
	}
	if hosts[0].Name != "prod" || hosts[0].User != "root" || hosts[0].Port != 2222 {
		t.Fatalf("unexpected prod host: %+v", hosts[0])
	}
	if hosts[1].Name != "backup" || hosts[2].Name != "backup-alt" {
		t.Fatalf("expected backup aliases to be expanded, got %+v", hosts)
	}
}

func TestAppConfigUpsertHostMergesByName(t *testing.T) {
	cfg := &AppConfig{}
	cfg.UpsertHost(model.HostConfig{Name: "prod", Kind: model.HostKindSSH, Host: "first.example"})
	cfg.UpsertHost(model.HostConfig{Name: "prod", Kind: model.HostKindSSH, Host: "second.example"})
	if len(cfg.Hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(cfg.Hosts))
	}
	if cfg.Hosts[0].Host != "second.example" {
		t.Fatalf("expected updated host, got %+v", cfg.Hosts[0])
	}
}
