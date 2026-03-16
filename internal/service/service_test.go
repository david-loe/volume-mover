package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/david-loe/volume-mover/internal/model"
	"github.com/david-loe/volume-mover/internal/shell"
)

type fakeRunner struct {
	outputs   map[string]string
	errs      map[string]error
	calls     []string
	pipeCalls []string
}

func (f *fakeRunner) Run(_ context.Context, host model.HostConfig, command string) (string, error) {
	key := host.Name + "::" + command
	f.calls = append(f.calls, key)
	if err, ok := f.errs[key]; ok {
		return "", err
	}
	return f.outputs[key], nil
}

func (f *fakeRunner) Pipe(_ context.Context, srcHost model.HostConfig, srcCommand string, dstHost model.HostConfig, dstCommand string) error {
	f.pipeCalls = append(f.pipeCalls, srcHost.Name+"::"+srcCommand+" => "+dstHost.Name+"::"+dstCommand)
	return nil
}

func newTestService(t *testing.T, runner *fakeRunner) *Service {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hosts.yaml")
	svc := New(path, runner)
	if err := svc.AddHost(model.HostConfig{Name: "remote", Kind: model.HostKindSSH, Host: "remote.example", Port: 22}); err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestValidateTransferRejectsSameTargetNameOnSameHost(t *testing.T) {
	err := ValidateTransfer(model.TransferRequest{
		Operation:         model.TransferClone,
		SourceHost:        "local",
		SourceVolume:      "data",
		DestinationHost:   "local",
		DestinationVolume: "data",
	})
	if err == nil || !strings.Contains(err.Error(), "must differ") {
		t.Fatalf("expected same-host validation error, got %v", err)
	}
}

func TestPlanTransferCommandsUsesHelperImage(t *testing.T) {
	plan := PlanTransferCommands(model.TransferRequest{SourceVolume: "src", DestinationVolume: "dst"}, "busybox:test")
	if !strings.Contains(plan.SourceCommand, "busybox:test") || !strings.Contains(plan.DestinationCommand, "busybox:test") {
		t.Fatalf("expected helper image in commands: %+v", plan)
	}
	if !strings.Contains(plan.SourceCommand, "'src':/from") || !strings.Contains(plan.DestinationCommand, "'dst':/to") {
		t.Fatalf("expected volume names in commands: %+v", plan)
	}
}

func TestTransferMoveDoesNotDeleteSourceWhenDestinationVerificationFails(t *testing.T) {
	runner := &fakeRunner{outputs: map[string]string{}, errs: map[string]error{}}
	svc := newTestService(t, runner)
	runner.outputs["local::docker volume inspect 'src'"] = `[{"Name":"src","Driver":"local","Labels":{}}]`
	runner.outputs["local::docker ps -aq --no-trunc"] = ""
	runner.outputs["local::"+sizeCommand("src")] = "10"
	runner.outputs["remote::docker volume inspect 'dst' >/dev/null 2>&1 && echo yes || echo no"] = "no"
	runner.outputs["remote::docker volume create 'dst'"] = "dst"
	runner.errs["remote::docker volume inspect 'dst'"] = errors.New("inspect failed")

	_, err := svc.Transfer(context.Background(), model.TransferRequest{
		Operation:         model.TransferMove,
		SourceHost:        "local",
		SourceVolume:      "src",
		DestinationHost:   "remote",
		DestinationVolume: "dst",
	})
	if err == nil || !strings.Contains(err.Error(), "verify destination volume") {
		t.Fatalf("expected destination verification error, got %v", err)
	}
	for _, call := range runner.calls {
		if strings.Contains(call, "docker volume rm 'src'") {
			t.Fatalf("source volume should not have been removed: %v", runner.calls)
		}
	}
}

func TestTransferQuiesceStopsAndRestartsContainersOnce(t *testing.T) {
	runner := &fakeRunner{outputs: map[string]string{}, errs: map[string]error{}}
	svc := newTestService(t, runner)
	containerID := "abcdef1234567890"
	inspectJSON := `[{"Id":"` + containerID + `","Name":"/app","State":{"Running":true,"Status":"running"},"Mounts":[{"Type":"volume","Name":"src"}]}]`
	runner.outputs["local::docker volume inspect 'src'"] = `[{"Name":"src","Driver":"local","Labels":{}}]`
	runner.outputs["local::docker ps -aq --no-trunc"] = containerID
	runner.outputs["local::docker inspect "+containerID] = inspectJSON
	runner.outputs["local::"+sizeCommand("src")] = "10"
	runner.outputs["local::docker volume inspect 'dst' >/dev/null 2>&1 && echo yes || echo no"] = "no"
	runner.outputs["local::docker volume create 'dst'"] = "dst"
	runner.outputs["local::docker stop 'abcdef123456'"] = "stopped"
	runner.outputs["local::docker start 'abcdef123456'"] = "started"
	runner.outputs["local::docker volume inspect 'dst'"] = `[{"Name":"dst","Driver":"local","Labels":{}}]`
	runner.outputs["local::"+sizeCommand("dst")] = "10"

	result, err := svc.Transfer(context.Background(), model.TransferRequest{
		Operation:         model.TransferCopy,
		SourceHost:        "local",
		SourceVolume:      "src",
		DestinationHost:   "local",
		DestinationVolume: "dst",
		AllowLive:         true,
		QuiesceSource:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" {
		t.Fatalf("expected success, got %+v", result)
	}
	stopCount := 0
	startCount := 0
	for _, call := range runner.calls {
		if strings.Contains(call, "docker stop 'abcdef123456'") {
			stopCount++
		}
		if strings.Contains(call, "docker start 'abcdef123456'") {
			startCount++
		}
	}
	if stopCount != 1 || startCount != 1 {
		t.Fatalf("expected exactly one stop and one start, got stop=%d start=%d calls=%v", stopCount, startCount, runner.calls)
	}
}

func TestParseVolumeSizeOutputAcceptsDockerPullNoise(t *testing.T) {
	out := "Unable to find image 'helper-image' locally\nPulling from library/helper-image\nlayer-1: Pulling fs layer\nlayer-1: Download complete\nlayer-1: Pull complete\nDigest: sha256:<redacted>\nStatus: Downloaded newer image for helper-image\n366036241"
	size, err := parseVolumeSizeOutput(out)
	if err != nil {
		t.Fatal(err)
	}
	if size != 366036241 {
		t.Fatalf("expected parsed size 366036241, got %d", size)
	}
}

func sizeCommand(volume string) string {
	return fmt.Sprintf("docker run --rm -v %s:/volume %s sh -c %s",
		shell.Quote(volume),
		shell.Quote("busybox:1.36"),
		shell.Quote("du -sb /volume 2>/dev/null | cut -f1 || du -sk /volume | awk '{print $1*1024}'"),
	)
}
