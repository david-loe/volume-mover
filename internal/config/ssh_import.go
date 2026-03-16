package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/david-loe/volume-mover/internal/model"
)

func ImportSSHHosts(path string) ([]model.HostConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open ssh config: %w", err)
	}
	defer file.Close()

	home, _ := os.UserHomeDir()
	scanner := bufio.NewScanner(file)
	var (
		aliases []string
		current = model.HostConfig{Kind: model.HostKindSSH, Imported: true}
		hosts   []model.HostConfig
	)
	flush := func() {
		if len(aliases) == 0 {
			return
		}
		for _, alias := range aliases {
			if alias == "" || strings.ContainsAny(alias, "*?") {
				continue
			}
			host := current
			host.Name = alias
			host.Alias = alias
			if host.Host == "" {
				host.Host = alias
			}
			hosts = append(hosts, host)
		}
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		key := strings.ToLower(parts[0])
		value := strings.Join(parts[1:], " ")
		switch key {
		case "host":
			flush()
			aliases = parts[1:]
			current = model.HostConfig{Kind: model.HostKindSSH, Imported: true}
		case "hostname":
			current.Host = value
		case "user":
			current.User = value
		case "port":
			port, err := strconv.Atoi(value)
			if err == nil {
				current.Port = port
			}
		case "identityfile":
			if strings.HasPrefix(value, "~/") && home != "" {
				value = filepath.Join(home, strings.TrimPrefix(value, "~/"))
			}
			current.IdentityFile = value
		}
	}
	flush()
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan ssh config: %w", err)
	}
	return hosts, nil
}
