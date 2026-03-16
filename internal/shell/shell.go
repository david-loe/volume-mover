package shell

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/david-loe/volume-mover/internal/model"
)

type Runner interface {
	Run(ctx context.Context, host model.HostConfig, command string) (string, error)
	Pipe(ctx context.Context, srcHost model.HostConfig, srcCommand string, dstHost model.HostConfig, dstCommand string) error
}

type CommandBuilder struct{}

func NewRunner() Runner {
	return &CommandBuilder{}
}

func (c *CommandBuilder) Run(ctx context.Context, host model.HostConfig, command string) (string, error) {
	cmd, cleanup, err := c.command(ctx, host, command)
	if err != nil {
		return "", err
	}
	defer cleanup()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run command on %s: %w: %s", host.Name, err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func (c *CommandBuilder) Pipe(ctx context.Context, srcHost model.HostConfig, srcCommand string, dstHost model.HostConfig, dstCommand string) error {
	src, srcCleanup, err := c.command(ctx, srcHost, srcCommand)
	if err != nil {
		return err
	}
	defer srcCleanup()
	dst, dstCleanup, err := c.command(ctx, dstHost, dstCommand)
	if err != nil {
		return err
	}
	defer dstCleanup()

	stdout, err := src.StdoutPipe()
	if err != nil {
		return fmt.Errorf("source stdout pipe: %w", err)
	}
	var srcErr bytes.Buffer
	var dstErr bytes.Buffer
	src.Stderr = &srcErr
	dst.Stderr = &dstErr
	dst.Stdin = stdout

	if err := dst.Start(); err != nil {
		return fmt.Errorf("start destination command: %w", err)
	}
	if err := src.Start(); err != nil {
		_ = dst.Process.Kill()
		return fmt.Errorf("start source command: %w", err)
	}

	srcRunErr := src.Wait()
	dstRunErr := dst.Wait()
	if srcRunErr != nil {
		return fmt.Errorf("source command failed: %w: %s", srcRunErr, strings.TrimSpace(srcErr.String()))
	}
	if dstRunErr != nil {
		return fmt.Errorf("destination command failed: %w: %s", dstRunErr, strings.TrimSpace(dstErr.String()))
	}
	return nil
}

func (c *CommandBuilder) command(ctx context.Context, host model.HostConfig, command string) (*exec.Cmd, func(), error) {
	if host.Kind == model.HostKindSSH {
		configPath, cleanup, err := sanitizedSSHConfig()
		if err != nil {
			return nil, nil, err
		}
		args := []string{"-F", configPath, "-o", "BatchMode=yes"}
		if host.Port > 0 {
			args = append(args, "-p", fmt.Sprintf("%d", host.Port))
		}
		if host.IdentityFile != "" {
			args = append(args, "-i", host.IdentityFile)
		}
		target := host.Host
		if host.Imported && host.Alias != "" {
			target = host.Alias
			if host.Host != "" && host.Host != host.Alias {
				args = append(args, "-o", "HostKeyAlias="+host.Host)
			}
		} else if target == "" {
			target = host.Alias
		}
		if host.User != "" {
			target = host.User + "@" + target
		}
		remote := "sh -lc " + Quote(command)
		args = append(args, target, remote)
		return exec.CommandContext(ctx, "ssh", args...), cleanup, nil
	}
	return exec.CommandContext(ctx, "bash", "-lc", command), func() {}, nil
}

func Quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

var sshConfigPath = defaultSSHConfigPath()

func defaultSSHConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join("/root", ".ssh", "config")
	}
	return filepath.Join(home, ".ssh", "config")
}

func sanitizedSSHConfig() (string, func(), error) {
	content, err := os.ReadFile(sshConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "/dev/null", func() {}, nil
		}
		return "", nil, fmt.Errorf("read ssh config: %w", err)
	}
	file, err := os.CreateTemp("", "volume-mover-ssh-*.config")
	if err != nil {
		return "", nil, fmt.Errorf("create temp ssh config: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", nil, fmt.Errorf("write temp ssh config: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", nil, fmt.Errorf("chmod temp ssh config: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", nil, fmt.Errorf("close temp ssh config: %w", err)
	}
	return file.Name(), func() {
		_ = os.Remove(file.Name())
	}, nil
}
