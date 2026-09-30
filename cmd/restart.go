package cmd

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(restartCmd)
}

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the weclaw bridge",
	RunE: func(cmd *cobra.Command, args []string) error {
		if serviceInstalled() {
			if err := serviceRestart(); err != nil {
				return err
			}
			if pid := waitServicePid(5 * time.Second); pid > 0 {
				fmt.Printf("weclaw service restarted (pid=%d)\n", pid)
			}
			return nil
		}

		// Stop if running
		pid, err := readPid()
		if err == nil && processExists(pid) {
			fmt.Printf("Stopping weclaw (pid=%d)...\n", pid)
			if p, err := os.FindProcess(pid); err == nil {
				p.Signal(syscall.SIGTERM)
			}
			for i := 0; i < 20; i++ {
				if !processExists(pid) {
					break
				}
				time.Sleep(500 * time.Millisecond)
			}
			os.Remove(pidFile())
		}

		// Start
		fmt.Println("Starting weclaw...")
		return runDaemon()
	},
}
