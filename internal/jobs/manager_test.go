package jobs

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/david-loe/volume-mover/internal/model"
	"github.com/david-loe/volume-mover/internal/service"
)

func TestValidateJobRequestRejectsDuplicateDestinations(t *testing.T) {
	err := validateJobRequest(model.CreateTransferJobRequest{
		Operation:       model.TransferCopy,
		SourceHost:      "local",
		DestinationHost: "remote",
		Items: []model.TransferJobItemRequest{
			{SourceVolume: "a", DestinationVolume: "dst"},
			{SourceVolume: "b", DestinationVolume: "dst"},
		},
	})
	if err == nil {
		t.Fatal("expected duplicate destination validation error")
	}
}

func TestCreateAndLoadJob(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "hosts.yaml")
	store, err := NewStore(filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager, err := NewManager(store, service.New(configPath, nil))
	if err != nil {
		t.Fatal(err)
	}
	job, err := manager.CreateJob(context.Background(), model.CreateTransferJobRequest{
		Operation:       model.TransferCopy,
		SourceHost:      "local",
		DestinationHost: "remote",
		Items: []model.TransferJobItemRequest{
			{SourceVolume: "alpha", DestinationVolume: "alpha-copy"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := manager.Job(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID == "" || loaded.SourceHost != "local" || len(loaded.Items) != 1 {
		t.Fatalf("unexpected job load: %+v", loaded)
	}
}
