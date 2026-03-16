package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/david-loe/volume-mover/internal/config"
	"github.com/david-loe/volume-mover/internal/model"
	"github.com/david-loe/volume-mover/internal/shell"
)

const defaultHelperImage = "busybox:1.36"

var trailingIntegerPattern = regexp.MustCompile(`(\d+)\s*$`)

type Service struct {
	configPath  string
	runner      shell.Runner
	helperImage string
}

type TransferCallbacks struct {
	OnStep      func(step string, message string)
	OnWarning   func(message string)
	CheckCancel func(step string) error
}

type dockerVolumeInspect struct {
	Name   string            `json:"Name"`
	Driver string            `json:"Driver"`
	Labels map[string]string `json:"Labels"`
}

type dockerContainerInspect struct {
	ID    string `json:"Id"`
	Name  string `json:"Name"`
	State struct {
		Running bool   `json:"Running"`
		Status  string `json:"Status"`
	} `json:"State"`
	Mounts []struct {
		Type   string `json:"Type"`
		Name   string `json:"Name"`
		Source string `json:"Source"`
	} `json:"Mounts"`
}

func New(configPath string, runner shell.Runner) *Service {
	helperImage := os.Getenv("VOLUME_MOVER_HELPER_IMAGE")
	if helperImage == "" {
		helperImage = defaultHelperImage
	}
	if runner == nil {
		runner = shell.NewRunner()
	}
	return &Service{configPath: configPath, runner: runner, helperImage: helperImage}
}

func (s *Service) ListHosts() ([]model.HostConfig, error) {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return nil, err
	}
	hosts := append([]model.HostConfig{localHost()}, cfg.Hosts...)
	sort.Slice(hosts, func(i, j int) bool { return hosts[i].Name < hosts[j].Name })
	return hosts, nil
}

func (s *Service) AddHost(host model.HostConfig) error {
	if err := validateHost(host); err != nil {
		return err
	}
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return err
	}
	cfg.UpsertHost(host)
	return config.Save(s.configPath, cfg)
}

func (s *Service) DeleteHost(name string) error {
	if name == localHost().Name {
		return errors.New("local host cannot be removed")
	}
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return err
	}
	cfg.DeleteHost(name)
	return config.Save(s.configPath, cfg)
}

func (s *Service) ImportSSHHosts() ([]model.HostConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	imported, err := config.ImportSSHHosts(home + "/.ssh/config")
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return nil, err
	}
	for _, host := range imported {
		cfg.UpsertHost(host)
	}
	if err := config.Save(s.configPath, cfg); err != nil {
		return nil, err
	}
	return imported, nil
}

func (s *Service) TestHost(ctx context.Context, name string) (string, error) {
	host, err := s.Host(name)
	if err != nil {
		return "", err
	}
	out, err := s.runner.Run(ctx, host, "docker version --format '{{.Server.Version}}'")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (s *Service) Host(name string) (model.HostConfig, error) {
	if name == "" || name == localHost().Name {
		return localHost(), nil
	}
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return model.HostConfig{}, err
	}
	if host, ok := cfg.FindHost(name); ok {
		return host, nil
	}
	return model.HostConfig{}, fmt.Errorf("host %q not found", name)
}

func (s *Service) ListVolumes(ctx context.Context, hostName string) ([]model.VolumeSummary, error) {
	host, err := s.Host(hostName)
	if err != nil {
		return nil, err
	}
	inspectByVolume, err := s.containerUsage(ctx, host)
	if err != nil {
		return nil, err
	}
	out, err := s.runner.Run(ctx, host, "docker volume ls --format '{{.Name}}|{{.Driver}}'")
	if err != nil {
		return nil, err
	}
	var volumes []model.VolumeSummary
	for _, line := range splitLines(out) {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		containers := inspectByVolume[parts[0]]
		volumes = append(volumes, model.VolumeSummary{
			Name:                  parts[0],
			Driver:                parts[1],
			AttachedContainers:    containers,
			AttachedContainersCnt: len(containers),
			RunningContainers:     runningCount(containers),
		})
	}
	sort.Slice(volumes, func(i, j int) bool { return volumes[i].Name < volumes[j].Name })
	return volumes, nil
}

func (s *Service) VolumeDetail(ctx context.Context, hostName, volumeName string) (model.VolumeDetail, error) {
	host, err := s.Host(hostName)
	if err != nil {
		return model.VolumeDetail{}, err
	}
	inspectOut, err := s.runner.Run(ctx, host, "docker volume inspect "+shell.Quote(volumeName))
	if err != nil {
		return model.VolumeDetail{}, err
	}
	var inspected []dockerVolumeInspect
	if err := json.Unmarshal([]byte(inspectOut), &inspected); err != nil {
		return model.VolumeDetail{}, fmt.Errorf("decode volume inspect: %w", err)
	}
	if len(inspected) == 0 {
		return model.VolumeDetail{}, fmt.Errorf("volume %q not found", volumeName)
	}
	usageByVolume, err := s.containerUsage(ctx, host)
	if err != nil {
		return model.VolumeDetail{}, err
	}
	sizeBytes, err := s.volumeSize(ctx, host, volumeName)
	if err != nil {
		return model.VolumeDetail{}, err
	}
	containers := usageByVolume[volumeName]
	return model.VolumeDetail{
		Summary: model.VolumeSummary{
			Name:                  inspected[0].Name,
			Driver:                inspected[0].Driver,
			Labels:                inspected[0].Labels,
			AttachedContainers:    containers,
			AttachedContainersCnt: len(containers),
			RunningContainers:     runningCount(containers),
		},
		SizeBytes:  sizeBytes,
		Containers: containers,
	}, nil
}

func (s *Service) Transfer(ctx context.Context, req model.TransferRequest) (model.TransferResult, error) {
	return s.TransferWithCallbacks(ctx, req, TransferCallbacks{})
}

func (s *Service) TransferWithCallbacks(ctx context.Context, req model.TransferRequest, callbacks TransferCallbacks) (model.TransferResult, error) {
	start := time.Now()
	result := model.TransferResult{Request: req, Status: "failed"}
	defer func() {
		result.Duration = time.Since(start)
	}()
	emitStep(callbacks, "validate", "validating transfer request")
	if err := ValidateTransfer(req); err != nil {
		return result, err
	}
	if err := checkCancelled(callbacks, "validate"); err != nil {
		return result, err
	}
	sourceHost, err := s.Host(req.SourceHost)
	if err != nil {
		return result, err
	}
	destinationHost, err := s.Host(req.DestinationHost)
	if err != nil {
		return result, err
	}
	emitStep(callbacks, "inspect-source", fmt.Sprintf("inspecting %s on %s", req.SourceVolume, sourceHost.Name))
	detail, err := s.VolumeDetail(ctx, sourceHost.Name, req.SourceVolume)
	if err != nil {
		return result, err
	}
	result.BytesCopied = detail.SizeBytes
	if detail.Summary.RunningContainers > 0 {
		warning := fmt.Sprintf("volume %s is attached to %d running containers", req.SourceVolume, detail.Summary.RunningContainers)
		result.Warnings = append(result.Warnings, warning)
		emitWarning(callbacks, warning)
		if !req.AllowLive {
			return result, fmt.Errorf("%s; use --allow-live to proceed", warning)
		}
	}
	emitStep(callbacks, "validate-destination", fmt.Sprintf("checking destination volume %s on %s", req.DestinationVolume, destinationHost.Name))
	if exists, err := s.volumeExists(ctx, destinationHost, req.DestinationVolume); err != nil {
		return result, err
	} else if exists {
		return result, fmt.Errorf("destination volume %q already exists on host %q", req.DestinationVolume, destinationHost.Name)
	}
	if err := checkCancelled(callbacks, "before-quiesce"); err != nil {
		return result, err
	}
	stopped := []model.ContainerRef{}
	restartStopped := false
	if req.QuiesceSource {
		if req.Operation == model.TransferMove {
			return result, errors.New("--quiesce-source is only supported for clone and copy")
		}
		emitStep(callbacks, "quiesce", "stopping running source containers")
		stopped, err = s.stopRunningContainers(ctx, sourceHost, detail.Containers)
		if err != nil {
			return result, err
		}
		restartStopped = len(stopped) > 0
		for _, container := range stopped {
			result.StoppedContainers = append(result.StoppedContainers, container.Name)
		}
		defer func() {
			if restartStopped {
				_ = s.startContainers(context.Background(), sourceHost, stopped)
			}
		}()
	}
	if err := checkCancelled(callbacks, "before-create-destination"); err != nil {
		return result, err
	}
	emitStep(callbacks, "create-destination", fmt.Sprintf("creating %s on %s", req.DestinationVolume, destinationHost.Name))
	if err := s.createVolume(ctx, destinationHost, req.DestinationVolume); err != nil {
		return result, err
	}
	emitStep(callbacks, "transfer", fmt.Sprintf("streaming %s to %s", req.SourceVolume, req.DestinationVolume))
	commands := PlanTransferCommands(req, s.helperImage)
	if err := s.runner.Pipe(ctx, sourceHost, commands.SourceCommand, destinationHost, commands.DestinationCommand); err != nil {
		return result, err
	}
	if err := checkCancelled(callbacks, "before-verify"); err != nil {
		return result, err
	}
	emitStep(callbacks, "verify-destination", fmt.Sprintf("verifying %s on %s", req.DestinationVolume, destinationHost.Name))
	if _, err := s.VolumeDetail(ctx, destinationHost.Name, req.DestinationVolume); err != nil {
		return result, fmt.Errorf("verify destination volume: %w", err)
	}
	if req.Operation == model.TransferMove {
		emitStep(callbacks, "cleanup-source", fmt.Sprintf("removing %s from %s", req.SourceVolume, sourceHost.Name))
		if err := s.removeVolume(ctx, sourceHost, req.SourceVolume); err != nil {
			result.SourceCleanup = err.Error()
			return result, fmt.Errorf("remove source volume: %w", err)
		}
		result.SourceCleanup = "removed"
	}
	if len(stopped) > 0 {
		emitStep(callbacks, "restart-source", "restarting previously stopped containers")
		if err := s.startContainers(ctx, sourceHost, stopped); err != nil {
			result.SourceCleanup = "restart failed"
			return result, fmt.Errorf("restart stopped containers: %w", err)
		}
		restartStopped = false
	}
	emitStep(callbacks, "complete", fmt.Sprintf("completed transfer of %s", req.SourceVolume))
	result.Status = "success"
	return result, nil
}

type TransferPlan struct {
	SourceCommand      string
	DestinationCommand string
}

func PlanTransferCommands(req model.TransferRequest, helperImage string) TransferPlan {
	sourceCommand := fmt.Sprintf("docker run --rm -v %s:/from %s sh -c %s",
		shell.Quote(req.SourceVolume),
		shell.Quote(helperImage),
		shell.Quote("cd /from && tar -cf - ."),
	)
	destinationCommand := fmt.Sprintf("docker run --rm -i -v %s:/to %s sh -c %s",
		shell.Quote(req.DestinationVolume),
		shell.Quote(helperImage),
		shell.Quote("cd /to && tar -xf -"),
	)
	return TransferPlan{SourceCommand: sourceCommand, DestinationCommand: destinationCommand}
}

func ValidateTransfer(req model.TransferRequest) error {
	if req.SourceHost == "" {
		return errors.New("source host is required")
	}
	if req.DestinationHost == "" {
		return errors.New("destination host is required")
	}
	if req.SourceVolume == "" {
		return errors.New("source volume is required")
	}
	if req.DestinationVolume == "" {
		return errors.New("destination volume is required")
	}
	if req.Operation != model.TransferClone && req.Operation != model.TransferCopy && req.Operation != model.TransferMove {
		return fmt.Errorf("unsupported operation %q", req.Operation)
	}
	if req.SourceHost == req.DestinationHost && req.SourceVolume == req.DestinationVolume {
		return errors.New("same-host destination volume must differ from source volume")
	}
	return nil
}

func validateHost(host model.HostConfig) error {
	if host.Name == "" {
		return errors.New("host name is required")
	}
	if host.Kind != model.HostKindLocal && host.Kind != model.HostKindSSH {
		return fmt.Errorf("unsupported host kind %q", host.Kind)
	}
	if host.Kind == model.HostKindSSH && host.Host == "" && host.Alias == "" {
		return errors.New("ssh host requires alias or hostname")
	}
	return nil
}

func localHost() model.HostConfig {
	return model.HostConfig{Name: "local", Kind: model.HostKindLocal, Host: "localhost"}
}

func splitLines(out string) []string {
	items := strings.Split(strings.ReplaceAll(strings.TrimSpace(out), "\r\n", "\n"), "\n")
	if len(items) == 1 && items[0] == "" {
		return nil
	}
	return items
}

func (s *Service) volumeExists(ctx context.Context, host model.HostConfig, name string) (bool, error) {
	out, err := s.runner.Run(ctx, host, "docker volume inspect "+shell.Quote(name)+" >/dev/null 2>&1 && echo yes || echo no")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "yes", nil
}

func (s *Service) createVolume(ctx context.Context, host model.HostConfig, name string) error {
	_, err := s.runner.Run(ctx, host, "docker volume create "+shell.Quote(name))
	return err
}

func (s *Service) removeVolume(ctx context.Context, host model.HostConfig, name string) error {
	_, err := s.runner.Run(ctx, host, "docker volume rm "+shell.Quote(name))
	return err
}

func (s *Service) volumeSize(ctx context.Context, host model.HostConfig, name string) (int64, error) {
	command := fmt.Sprintf("docker run --rm -v %s:/volume %s sh -c %s",
		shell.Quote(name),
		shell.Quote(s.helperImage),
		shell.Quote("du -sb /volume 2>/dev/null | cut -f1 || du -sk /volume | awk '{print $1*1024}'"),
	)
	out, err := s.runner.Run(ctx, host, command)
	if err != nil {
		return 0, err
	}
	return parseVolumeSizeOutput(out)
}

func (s *Service) containerUsage(ctx context.Context, host model.HostConfig) (map[string][]model.ContainerRef, error) {
	idsOut, err := s.runner.Run(ctx, host, "docker ps -aq --no-trunc")
	if err != nil {
		return nil, err
	}
	ids := splitLines(idsOut)
	usage := map[string][]model.ContainerRef{}
	if len(ids) == 0 {
		return usage, nil
	}
	inspectCommand := "docker inspect " + strings.Join(ids, " ")
	out, err := s.runner.Run(ctx, host, inspectCommand)
	if err != nil {
		return nil, err
	}
	var containers []dockerContainerInspect
	if err := json.Unmarshal([]byte(out), &containers); err != nil {
		return nil, fmt.Errorf("decode container inspect: %w", err)
	}
	for _, container := range containers {
		ref := model.ContainerRef{
			ID:      shortID(container.ID),
			Name:    strings.TrimPrefix(container.Name, "/"),
			Status:  container.State.Status,
			Running: container.State.Running,
		}
		for _, mount := range container.Mounts {
			if mount.Type == "volume" && mount.Name != "" {
				usage[mount.Name] = append(usage[mount.Name], ref)
			}
		}
	}
	for name := range usage {
		sort.Slice(usage[name], func(i, j int) bool { return usage[name][i].Name < usage[name][j].Name })
	}
	return usage, nil
}

func (s *Service) stopRunningContainers(ctx context.Context, host model.HostConfig, containers []model.ContainerRef) ([]model.ContainerRef, error) {
	var stopped []model.ContainerRef
	for _, container := range containers {
		if !container.Running {
			continue
		}
		if _, err := s.runner.Run(ctx, host, "docker stop "+shell.Quote(container.ID)); err != nil {
			return nil, err
		}
		stopped = append(stopped, container)
	}
	return stopped, nil
}

func (s *Service) startContainers(ctx context.Context, host model.HostConfig, containers []model.ContainerRef) error {
	for _, container := range containers {
		if _, err := s.runner.Run(ctx, host, "docker start "+shell.Quote(container.ID)); err != nil {
			return err
		}
	}
	return nil
}

func runningCount(containers []model.ContainerRef) int {
	count := 0
	for _, container := range containers {
		if container.Running {
			count++
		}
	}
	return count
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func parseVolumeSizeOutput(out string) (int64, error) {
	value := strings.TrimSpace(out)
	if value == "" {
		return 0, nil
	}
	match := trailingIntegerPattern.FindStringSubmatch(value)
	if len(match) != 2 {
		return 0, fmt.Errorf("parse size %q: expected trailing integer", value)
	}
	var size int64
	_, err := fmt.Sscan(match[1], &size)
	if err != nil {
		return 0, fmt.Errorf("parse size %q: %w", value, err)
	}
	return size, nil
}

func emitStep(callbacks TransferCallbacks, step string, message string) {
	if callbacks.OnStep != nil {
		callbacks.OnStep(step, message)
	}
}

func emitWarning(callbacks TransferCallbacks, message string) {
	if callbacks.OnWarning != nil {
		callbacks.OnWarning(message)
	}
}

func checkCancelled(callbacks TransferCallbacks, step string) error {
	if callbacks.CheckCancel != nil {
		return callbacks.CheckCancel(step)
	}
	return nil
}
