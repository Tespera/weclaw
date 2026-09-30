package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

// SourceDir is the source checkout this binary was built from, set at build
// time via -ldflags. `weclaw update` rebuilds and reinstalls from it.
var SourceDir = ""

func init() {
	rootCmd.AddCommand(updateCmd)
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Rebuild and reinstall weclaw from its source checkout",
	Long: `Rebuild weclaw from the source checkout it was built from and reinstall it.

If the checkout's branch tracks a remote, it is fast-forwarded first. The
install step replaces the binary atomically and restarts a running bridge.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		src, err := updateSourceDir()
		if err != nil {
			return err
		}
		fmt.Printf("Source: %s\n", src)

		if run(src, "git", "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}") == nil {
			fmt.Println("Pulling latest changes...")
			if err := runVisible(src, "git", "pull", "--ff-only"); err != nil {
				return fmt.Errorf("git pull: %w", err)
			}
		} else {
			fmt.Println("No upstream remote configured; building local HEAD.")
		}

		fmt.Println("Building and installing...")
		if err := runVisible(src, "make", "install"); err != nil {
			return fmt.Errorf("make install: %w", err)
		}
		return nil
	},
}

func updateSourceDir() (string, error) {
	if SourceDir == "" {
		return "", errors.New("this binary was not built with a source directory; build it with 'make install' from the weclaw checkout")
	}
	if _, err := os.Stat(filepath.Join(SourceDir, "go.mod")); err != nil {
		return "", fmt.Errorf("source checkout not found at %s: %w", SourceDir, err)
	}
	return SourceDir, nil
}

func run(dir, name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Dir = dir
	return c.Run()
}

func runVisible(dir, name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Dir = dir
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	return c.Run()
}
