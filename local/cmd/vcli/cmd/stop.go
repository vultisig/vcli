package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func NewStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop all local development services",
		Long:  `Stop all local development services (calls 'make stop').`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMakeStop()
		},
	}

	return cmd
}

func runMakeStop() error {
	fmt.Println("Running: make stop")

	vcliDir := findVcliRoot()

	makeCmd := exec.Command("make", "stop")
	makeCmd.Dir = vcliDir
	makeCmd.Stdout = os.Stdout
	makeCmd.Stderr = os.Stderr

	return makeCmd.Run()
}
