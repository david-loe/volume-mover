package model

import "time"

type HostKind string

const (
	HostKindLocal HostKind = "local"
	HostKindSSH   HostKind = "ssh"
)

type HostConfig struct {
	Name         string   `yaml:"name" json:"name"`
	Kind         HostKind `yaml:"kind" json:"kind"`
	Alias        string   `yaml:"alias,omitempty" json:"alias,omitempty"`
	Host         string   `yaml:"host,omitempty" json:"host,omitempty"`
	User         string   `yaml:"user,omitempty" json:"user,omitempty"`
	Port         int      `yaml:"port,omitempty" json:"port,omitempty"`
	IdentityFile string   `yaml:"identity_file,omitempty" json:"identity_file,omitempty"`
	Imported     bool     `yaml:"imported,omitempty" json:"imported,omitempty"`
}

type VolumeSummary struct {
	Name                  string            `json:"name"`
	Driver                string            `json:"driver"`
	Labels                map[string]string `json:"labels,omitempty"`
	AttachedContainers    []ContainerRef    `json:"attached_containers,omitempty"`
	RunningContainers     int               `json:"running_containers"`
	AttachedContainersCnt int               `json:"attached_containers_count"`
}

type VolumeDetail struct {
	Summary    VolumeSummary  `json:"summary"`
	SizeBytes  int64          `json:"size_bytes"`
	Containers []ContainerRef `json:"containers"`
}

type ContainerRef struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Running bool   `json:"running"`
}

type TransferOperation string

const (
	TransferClone TransferOperation = "clone"
	TransferCopy  TransferOperation = "copy"
	TransferMove  TransferOperation = "move"
)

type TransferRequest struct {
	Operation         TransferOperation `json:"operation"`
	SourceHost        string            `json:"source_host"`
	SourceVolume      string            `json:"source_volume"`
	DestinationHost   string            `json:"destination_host"`
	DestinationVolume string            `json:"destination_volume"`
	AllowLive         bool              `json:"allow_live"`
	QuiesceSource     bool              `json:"quiesce_source"`
}

type TransferResult struct {
	Request           TransferRequest `json:"request"`
	Status            string          `json:"status"`
	BytesCopied       int64           `json:"bytes_copied"`
	Duration          time.Duration   `json:"duration"`
	Warnings          []string        `json:"warnings,omitempty"`
	SourceCleanup     string          `json:"source_cleanup,omitempty"`
	StoppedContainers []string        `json:"stopped_containers,omitempty"`
}
