package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/david-loe/volume-mover/internal/config"
	"github.com/david-loe/volume-mover/internal/humanize"
	"github.com/david-loe/volume-mover/internal/model"
	"github.com/david-loe/volume-mover/internal/service"
	"github.com/david-loe/volume-mover/internal/web"
	"github.com/spf13/cobra"
)

type rootOptions struct {
	configPath string
}

func NewRootCmd() *cobra.Command {
	opts := &rootOptions{}
	cmd := &cobra.Command{
		Use:   "volume-mover",
		Short: "Clone, copy, and move Docker volumes across hosts",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.configPath != "" {
				return nil
			}
			path, err := config.DefaultConfigPath()
			if err != nil {
				return err
			}
			opts.configPath = path
			return nil
		},
	}
	cmd.PersistentFlags().StringVar(&opts.configPath, "config", "", "path to host config")
	cmd.AddCommand(newHostCmd(opts))
	cmd.AddCommand(newVolumeCmd(opts))
	cmd.AddCommand(newWebCmd(opts))
	return cmd
}

func appService(opts *rootOptions) *service.Service {
	return service.New(opts.configPath, nil)
}

func contextWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Minute)
}

func renderHosts(hosts []model.HostConfig) string {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tKIND\tTARGET\tUSER\tPORT\tIDENTITY")
	for _, host := range hosts {
		target := host.Host
		if host.Alias != "" {
			target = host.Alias
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n", host.Name, host.Kind, target, host.User, host.Port, host.IdentityFile)
	}
	_ = w.Flush()
	return b.String()
}

func renderVolumes(volumes []model.VolumeSummary) string {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tDRIVER\tATTACHED\tRUNNING")
	for _, volume := range volumes {
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\n", volume.Name, volume.Driver, volume.AttachedContainersCnt, volume.RunningContainers)
	}
	_ = w.Flush()
	return b.String()
}

func printErr(err error) {
	fmt.Fprintln(os.Stderr, err)
}

type transferFlags struct {
	sourceHost        string
	sourceVolume      string
	destinationHost   string
	destinationVolume string
	allowLive         bool
	quiesceSource     bool
}

func (f transferFlags) request(op model.TransferOperation) model.TransferRequest {
	return model.TransferRequest{
		Operation:         op,
		SourceHost:        f.sourceHost,
		SourceVolume:      f.sourceVolume,
		DestinationHost:   f.destinationHost,
		DestinationVolume: f.destinationVolume,
		AllowLive:         f.allowLive,
		QuiesceSource:     f.quiesceSource,
	}
}

func newHostCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "host", Short: "Manage Docker hosts"}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List configured hosts",
		RunE: func(cmd *cobra.Command, args []string) error {
			hosts, err := appService(opts).ListHosts()
			if err != nil {
				return err
			}
			fmt.Print(renderHosts(hosts))
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "import-ssh",
		Short: "Import hosts from ~/.ssh/config",
		RunE: func(cmd *cobra.Command, args []string) error {
			hosts, err := appService(opts).ImportSSHHosts()
			if err != nil {
				return err
			}
			fmt.Printf("imported %d hosts\n", len(hosts))
			return nil
		},
	})
	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Add or update a host",
		RunE: func(cmd *cobra.Command, args []string) error {
			host := model.HostConfig{}
			host.Name, _ = cmd.Flags().GetString("name")
			kind, _ := cmd.Flags().GetString("kind")
			host.Kind = model.HostKind(kind)
			host.Host, _ = cmd.Flags().GetString("host")
			host.User, _ = cmd.Flags().GetString("user")
			host.Alias, _ = cmd.Flags().GetString("alias")
			host.IdentityFile, _ = cmd.Flags().GetString("identity-file")
			host.Port, _ = cmd.Flags().GetInt("port")
			if err := appService(opts).AddHost(host); err != nil {
				return err
			}
			fmt.Printf("saved host %s\n", host.Name)
			return nil
		},
	}
	addCmd.Flags().String("name", "", "host name")
	addCmd.Flags().String("kind", string(model.HostKindSSH), "host kind: local or ssh")
	addCmd.Flags().String("host", "", "hostname or IP")
	addCmd.Flags().String("user", "", "ssh user")
	addCmd.Flags().String("alias", "", "ssh config alias")
	addCmd.Flags().Int("port", 22, "ssh port")
	addCmd.Flags().String("identity-file", "", "ssh key path")
	_ = addCmd.MarkFlagRequired("name")
	cmd.AddCommand(addCmd)
	cmd.AddCommand(&cobra.Command{
		Use:   "test <name>",
		Short: "Test Docker connectivity to a host",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := contextWithTimeout()
			defer cancel()
			version, err := appService(opts).TestHost(ctx, args[0])
			if err != nil {
				return err
			}
			fmt.Printf("%s: docker server %s\n", args[0], version)
			return nil
		},
	})
	return cmd
}

func newVolumeCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "volume", Short: "Inspect and move Docker volumes"}
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List volumes on a host",
		RunE: func(cmd *cobra.Command, args []string) error {
			host, _ := cmd.Flags().GetString("host")
			ctx, cancel := contextWithTimeout()
			defer cancel()
			volumes, err := appService(opts).ListVolumes(ctx, host)
			if err != nil {
				return err
			}
			fmt.Print(renderVolumes(volumes))
			return nil
		},
	}
	listCmd.Flags().String("host", "local", "host name")
	cmd.AddCommand(listCmd)
	showCmd := &cobra.Command{
		Use:   "show <volume>",
		Short: "Show volume details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			host, _ := cmd.Flags().GetString("host")
			ctx, cancel := contextWithTimeout()
			defer cancel()
			detail, err := appService(opts).VolumeDetail(ctx, host, args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Name: %s\nDriver: %s\nSize: %s (%d bytes)\nAttached Containers: %d\nRunning Containers: %d\n", detail.Summary.Name, detail.Summary.Driver, humanize.Bytes(detail.SizeBytes), detail.SizeBytes, detail.Summary.AttachedContainersCnt, detail.Summary.RunningContainers)
			if len(detail.Containers) > 0 {
				fmt.Println("Containers:")
				for _, container := range detail.Containers {
					fmt.Printf("- %s (%s) running=%t\n", container.Name, container.ID, container.Running)
				}
			}
			return nil
		},
	}
	showCmd.Flags().String("host", "local", "host name")
	cmd.AddCommand(showCmd)
	cmd.AddCommand(newTransferCmd(opts, "clone", model.TransferClone, true))
	cmd.AddCommand(newTransferCmd(opts, "copy", model.TransferCopy, false))
	cmd.AddCommand(newTransferCmd(opts, "move", model.TransferMove, false))
	return cmd
}

func newTransferCmd(opts *rootOptions, use string, op model.TransferOperation, sameHost bool) *cobra.Command {
	flags := &transferFlags{}
	cmd := &cobra.Command{
		Use:   use,
		Short: strings.Title(use) + " a Docker volume",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := contextWithTimeout()
			defer cancel()
			if sameHost {
				flags.destinationHost = flags.sourceHost
			}
			result, err := appService(opts).Transfer(ctx, flags.request(op))
			if err != nil {
				return err
			}
			fmt.Printf("status=%s size=%s (%d bytes) duration=%s\n", result.Status, humanize.Bytes(result.BytesCopied), result.BytesCopied, result.Duration)
			if len(result.Warnings) > 0 {
				fmt.Println(strings.Join(result.Warnings, "\n"))
			}
			return nil
		},
	}
	if sameHost {
		cmd.Flags().StringVar(&flags.sourceHost, "host", "local", "host name")
		cmd.Flags().StringVar(&flags.sourceVolume, "source", "", "source volume")
		cmd.Flags().StringVar(&flags.destinationVolume, "dest", "", "destination volume")
	} else {
		cmd.Flags().StringVar(&flags.sourceHost, "source-host", "local", "source host")
		cmd.Flags().StringVar(&flags.sourceVolume, "source-volume", "", "source volume")
		cmd.Flags().StringVar(&flags.destinationHost, "dest-host", "local", "destination host")
		cmd.Flags().StringVar(&flags.destinationVolume, "dest-volume", "", "destination volume")
	}
	cmd.Flags().BoolVar(&flags.allowLive, "allow-live", false, "allow transfers of live volumes with warnings")
	cmd.Flags().BoolVar(&flags.quiesceSource, "quiesce-source", false, "stop source containers for clone/copy and restart them afterwards")
	return cmd
}

func newWebCmd(opts *rootOptions) *cobra.Command {
	var listen string
	cmd := &cobra.Command{
		Use:   "web",
		Short: "Run the embedded web UI",
		RunE: func(cmd *cobra.Command, args []string) error {
			server, err := web.New(appService(opts), opts.configPath, listen)
			if err != nil {
				return err
			}
			fmt.Printf("web UI listening on %s\n", listen)
			return server.Run()
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:8080", "listen address")
	return cmd
}
