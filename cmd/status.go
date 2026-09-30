package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether the weclaw bridge is running",
	RunE: func(cmd *cobra.Command, args []string) error {
		mode := "background"
		if serviceInstalled() {
			mode = "service (" + plistPath() + ")"
		}
		// The instance lock is authoritative: it is held exactly while a bridge runs.
		if pid, running := lockHolderPid(lockFile()); running {
			fmt.Printf("weclaw is running (pid=%d, mode=%s)\n", pid, mode)
			fmt.Printf("Log: %s\n", logFile())
			return nil
		}
		if pid, err := readPid(); err == nil && processExists(pid) {
			fmt.Printf("weclaw is running (pid=%d, mode=%s)\n", pid, mode)
			fmt.Printf("Log: %s\n", logFile())
			return nil
		}
		fmt.Printf("weclaw is not running (mode=%s)\n", mode)
		return nil
	},
}
