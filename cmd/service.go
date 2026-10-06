package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	serviceCmd.AddCommand(serviceInstallCmd, serviceUninstallCmd, serviceReloadCmd)
	rootCmd.AddCommand(serviceCmd)
}

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Run weclaw as a login service (launchd on macOS)",
	Long: `Install weclaw as a LaunchAgent so it starts at login and restarts on crash.

Once installed, weclaw start/stop/restart/status manage the service through
launchctl instead of spawning a separate background process.`,
}

var serviceInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install and start the login service for this binary",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := serviceInstall(); err != nil {
			return err
		}
		fmt.Printf("Service installed: %s\n", plistPath())
		if pid := waitServicePid(5 * time.Second); pid > 0 {
			fmt.Printf("weclaw is running (pid=%d)\n", pid)
		}
		fmt.Printf("Log: %s\n", logFile())
		return nil
	},
}

// serviceReloadCmd is what `weclaw restart` hands off to when it runs inside the
// service; it is not meant to be typed.
var serviceReloadCmd = &cobra.Command{
	Use:    "reload",
	Short:  "Reload the login service (bootout and bootstrap)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := serviceReload(); err != nil {
			return err
		}
		if pid := waitServicePid(5 * time.Second); pid > 0 {
			fmt.Printf("weclaw service reloaded (pid=%d)\n", pid)
		}
		return nil
	},
}

var serviceUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Stop and remove the login service",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := serviceUninstall(); err != nil {
			return err
		}
		fmt.Println("Service uninstalled")
		return nil
	},
}
