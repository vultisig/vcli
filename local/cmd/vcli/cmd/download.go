package cmd

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	goWrappersURL     = "https://github.com/vultisig/go-wrappers/archive/refs/heads/master.tar.gz"
	goWrappersVersion = "master" // Can be updated to use specific releases
)

// EnsureGoWrappers downloads the go-wrappers CGO libraries if not already present.
// Returns the path to the platform-specific library directory.
func EnsureGoWrappers() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}

	libDir := filepath.Join(home, ".vultisig", "lib")

	// Platform-specific subdirectory (darwin or linux)
	platform := runtime.GOOS
	if platform != "darwin" && platform != "linux" {
		return "", fmt.Errorf("unsupported platform: %s (only darwin and linux are supported)", platform)
	}

	platformDir := filepath.Join(libDir, platform)

	// Check if already downloaded
	marker := filepath.Join(platformDir, ".downloaded-"+goWrappersVersion)
	if _, err := os.Stat(marker); err == nil {
		// Already downloaded
		return platformDir, nil
	}

	// Create lib directory
	if err := os.MkdirAll(libDir, 0755); err != nil {
		return "", fmt.Errorf("create lib dir: %w", err)
	}

	fmt.Println("Downloading go-wrappers libraries...")
	fmt.Printf("  Source: %s\n", goWrappersURL)
	fmt.Printf("  Target: %s\n", platformDir)

	// Download the tarball
	resp, err := http.Get(goWrappersURL)
	if err != nil {
		return "", fmt.Errorf("download go-wrappers: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status: %s", resp.Status)
	}

	// Extract the tarball
	gzr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return "", fmt.Errorf("create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	// Look for includes/<platform>/ files
	includesPrefix := "go-wrappers-master/includes/"
	platformPrefix := includesPrefix + platform + "/"
	headerPrefix := includesPrefix

	extractedFiles := 0

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read tar: %w", err)
		}

		// Extract platform-specific libraries
		if strings.HasPrefix(header.Name, platformPrefix) && header.Typeflag == tar.TypeReg {
			filename := filepath.Base(header.Name)
			targetPath := filepath.Join(platformDir, filename)

			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return "", fmt.Errorf("create dir for %s: %w", filename, err)
			}

			outFile, err := os.Create(targetPath)
			if err != nil {
				return "", fmt.Errorf("create file %s: %w", filename, err)
			}

			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return "", fmt.Errorf("write file %s: %w", filename, err)
			}
			outFile.Close()

			// Make libraries executable
			if err := os.Chmod(targetPath, 0755); err != nil {
				return "", fmt.Errorf("chmod %s: %w", filename, err)
			}

			fmt.Printf("  Extracted: %s\n", filename)
			extractedFiles++
		}

		// Also extract header files to the lib root
		if strings.HasPrefix(header.Name, headerPrefix) && strings.HasSuffix(header.Name, ".h") && header.Typeflag == tar.TypeReg {
			filename := filepath.Base(header.Name)
			targetPath := filepath.Join(libDir, filename)

			outFile, err := os.Create(targetPath)
			if err != nil {
				return "", fmt.Errorf("create header %s: %w", filename, err)
			}

			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return "", fmt.Errorf("write header %s: %w", filename, err)
			}
			outFile.Close()
		}
	}

	if extractedFiles == 0 {
		return "", fmt.Errorf("no library files found for platform %s", platform)
	}

	// Write marker file
	if err := os.WriteFile(marker, []byte(goWrappersVersion), 0644); err != nil {
		return "", fmt.Errorf("write marker: %w", err)
	}

	fmt.Printf("  Done! Extracted %d files\n", extractedFiles)

	return platformDir, nil
}

// GetLibraryPath returns the path to go-wrappers libraries without downloading.
// Returns empty string if not yet downloaded.
func GetLibraryPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	platform := runtime.GOOS
	platformDir := filepath.Join(home, ".vultisig", "lib", platform)

	marker := filepath.Join(platformDir, ".downloaded-"+goWrappersVersion)
	if _, err := os.Stat(marker); err != nil {
		return ""
	}

	return platformDir
}

// SetupLibraryPath ensures go-wrappers is downloaded and sets the library path environment variables.
// This should be called early in main() before any CGO code is loaded.
func SetupLibraryPath() error {
	libPath, err := EnsureGoWrappers()
	if err != nil {
		return err
	}

	// Set environment variables for CGO
	currentDyld := os.Getenv("DYLD_LIBRARY_PATH")
	currentLd := os.Getenv("LD_LIBRARY_PATH")

	if currentDyld == "" {
		os.Setenv("DYLD_LIBRARY_PATH", libPath)
	} else if !strings.Contains(currentDyld, libPath) {
		os.Setenv("DYLD_LIBRARY_PATH", libPath+":"+currentDyld)
	}

	if currentLd == "" {
		os.Setenv("LD_LIBRARY_PATH", libPath)
	} else if !strings.Contains(currentLd, libPath) {
		os.Setenv("LD_LIBRARY_PATH", libPath+":"+currentLd)
	}

	return nil
}
