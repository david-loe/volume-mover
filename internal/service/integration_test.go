package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/david-loe/volume-mover/internal/model"
)

func TestIntegrationSameHostCopy(t *testing.T) {
	if os.Getenv("VOLUME_MOVER_RUN_DOCKER_TESTS") != "1" {
		t.Skip("set VOLUME_MOVER_RUN_DOCKER_TESTS=1 to run Docker integration tests")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	svc := New(filepath.Join(t.TempDir(), "hosts.yaml"), nil)
	src := "vm-int-src"
	dst := "vm-int-dst"
	cleanupDockerVolume(t, src)
	cleanupDockerVolume(t, dst)
	mustRun(t, "docker", "volume", "create", src)
	mustRun(t, "docker", "run", "--rm", "-v", src+":/data", "busybox:1.36", "sh", "-c", "echo integration > /data/file.txt")
	defer cleanupDockerVolume(t, src)
	defer cleanupDockerVolume(t, dst)

	result, err := svc.Transfer(context.Background(), model.TransferRequest{
		Operation:         model.TransferCopy,
		SourceHost:        "local",
		SourceVolume:      src,
		DestinationHost:   "local",
		DestinationVolume: dst,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" {
		t.Fatalf("expected success, got %+v", result)
	}
	out := mustRun(t, "docker", "run", "--rm", "-v", dst+":/data", "busybox:1.36", "cat", "/data/file.txt")
	if !strings.Contains(out, "integration") {
		t.Fatalf("expected copied data, got %q", out)
	}
}

func cleanupDockerVolume(t *testing.T, name string) {
	t.Helper()
	_ = exec.Command("docker", "volume", "rm", "-f", name).Run()
}

func mustRun(t *testing.T, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command %s %v failed: %v\n%s", name, args, err, out)
	}
	return string(out)
}
