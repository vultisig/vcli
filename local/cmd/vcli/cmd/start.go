package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

// Build mode constants
const (
	BuildLocal  = "local"  // Infra in Docker, services run natively
	BuildVolume = "volume" // Full stack in Docker with volume mounts
	BuildImage  = "image"  // Full stack in Docker with GHCR images
)

func NewStartCmd() *cobra.Command {
	var buildMode string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start all local development services",
		Long: `Start all local development services.

BUILD MODES:
  --build=local   Infra in Docker + services run natively with go run (default)
  --build=volume  Full stack in Docker with volume mounts + hot-reload
  --build=image   Full stack in Docker with GHCR images (no repos needed)
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMakeStart(buildMode)
		},
	}

	cmd.Flags().StringVar(&buildMode, "build", BuildLocal, "Build mode: local, volume, image")

	return cmd
}

func runMakeStart(buildMode string) error {
	fmt.Printf("Running: make start build=%s\n\n", buildMode)

	vcliDir := findVcliRoot()

	makeCmd := exec.Command("make", "start", fmt.Sprintf("build=%s", buildMode))
	makeCmd.Dir = vcliDir
	makeCmd.Stdout = os.Stdout
	makeCmd.Stderr = os.Stderr
	makeCmd.Stdin = os.Stdin

	return makeCmd.Run()
}

func findVcliRoot() string {
	paths := []string{".", "..", "../.."}

	for _, p := range paths {
		makefile := filepath.Join(p, "Makefile")
		if _, err := os.Stat(makefile); err == nil {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}

	return ".."
}

// findLocalDir finds the local directory containing docker-compose files
func findLocalDir() string {
	paths := []string{".", "local"}

	for _, p := range paths {
		dockerCompose := filepath.Join(p, "docker-compose.yaml")
		dockerComposeFull := filepath.Join(p, "docker-compose.full.yaml")
		if _, err := os.Stat(dockerCompose); err == nil {
			abs, _ := filepath.Abs(p)
			return abs
		}
		if _, err := os.Stat(dockerComposeFull); err == nil {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}

	return "local"
}
