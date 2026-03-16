package shell

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
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
	cmd := c.command(ctx, host, command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run command on %s: %w: %s", host.Name, err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func (c *CommandBuilder) Pipe(ctx context.Context, srcHost model.HostConfig, srcCommand string, dstHost model.HostConfig, dstCommand string) error {
	src := c.command(ctx, srcHost, srcCommand)
	dst := c.command(ctx, dstHost, dstCommand)

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

func (c *CommandBuilder) command(ctx context.Context, host model.HostConfig, command string) *exec.Cmd {
	if host.Kind == model.HostKindSSH {
		args := []string{"-o", "BatchMode=yes"}
		if host.Port > 0 {
			args = append(args, "-p", fmt.Sprintf("%d", host.Port))
		}
		if host.IdentityFile != "" {
			args = append(args, "-i", host.IdentityFile)
		}
		target := host.Alias
		if target == "" {
			target = host.Host
			if host.User != "" {
				target = host.User + "@" + target
			}
		}
		remote := "sh -lc " + Quote(command)
		args = append(args, target, remote)
		return exec.CommandContext(ctx, "ssh", args...)
	}
	return exec.CommandContext(ctx, "bash", "-lc", command)
}

func Quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
