package main

import (
	"fmt"
	"os"

	"github.com/david-loe/volume-mover/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
