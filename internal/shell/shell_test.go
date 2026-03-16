package shell

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/david-loe/volume-mover/internal/model"
)

func TestCommandUsesSanitizedSSHConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	if err := os.WriteFile(configPath, []byte("Host remote\n  StrictHostKeyChecking accept-new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	original := sshConfigPath
	sshConfigPath = configPath
	defer func() {
		sshConfigPath = original
	}()

	cmd, cleanup, err := (&CommandBuilder{}).command(context.Background(), model.HostConfig{
		Name:         "remote",
		Kind:         model.HostKindSSH,
		Host:         "remote.example",
		User:         "root",
		Port:         2222,
		IdentityFile: "/root/.ssh/id_ed25519",
	}, "docker version")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	gotConfig := cmd.Args[2]
	if gotConfig == configPath || gotConfig == "/dev/null" {
		t.Fatalf("expected sanitized temp config, got %q", gotConfig)
	}
	info, err := os.Stat(gotConfig)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected temp config perms 0600, got %o", info.Mode().Perm())
	}
	content, err := os.ReadFile(gotConfig)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "Host remote\n  StrictHostKeyChecking accept-new\n" {
		t.Fatalf("unexpected temp config content: %q", string(content))
	}

	want := []string{
		"ssh",
		"-F", gotConfig,
		"-o", "BatchMode=yes",
		"-p", "2222",
		"-i", "/root/.ssh/id_ed25519",
		"root@remote.example",
		"sh -lc 'docker version'",
	}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("unexpected command args:\n got: %#v\nwant: %#v", cmd.Args, want)
	}
	cleanup()
	if _, err := os.Stat(gotConfig); !os.IsNotExist(err) {
		t.Fatalf("expected temp config to be removed, stat err=%v", err)
	}
}

func TestCommandFallsBackToDevNullWhenSSHConfigMissing(t *testing.T) {
	original := sshConfigPath
	sshConfigPath = filepath.Join(t.TempDir(), "missing-config")
	defer func() {
		sshConfigPath = original
	}()

	cmd, cleanup, err := (&CommandBuilder{}).command(context.Background(), model.HostConfig{
		Name: "remote",
		Kind: model.HostKindSSH,
		Host: "remote.example",
	}, "docker version")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	want := []string{
		"ssh",
		"-F", "/dev/null",
		"-o", "BatchMode=yes",
		"remote.example",
		"sh -lc 'docker version'",
	}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("unexpected command args:\n got: %#v\nwant: %#v", cmd.Args, want)
	}
}

func TestImportedCommandUsesAliasAndHostKeyAlias(t *testing.T) {
	original := sshConfigPath
	sshConfigPath = filepath.Join(t.TempDir(), "missing-config")
	defer func() {
		sshConfigPath = original
	}()

	cmd, cleanup, err := (&CommandBuilder{}).command(context.Background(), model.HostConfig{
		Name:     "example-remote",
		Kind:     model.HostKindSSH,
		Imported: true,
		Alias:    "example-remote",
		Host:     "example.internal",
		User:     "root",
	}, "docker version")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	want := []string{
		"ssh",
		"-F", "/dev/null",
		"-o", "BatchMode=yes",
		"-o", "HostKeyAlias=example.internal",
		"root@example-remote",
		"sh -lc 'docker version'",
	}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("unexpected command args:\n got: %#v\nwant: %#v", cmd.Args, want)
	}
}
