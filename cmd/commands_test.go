package cmd

import "testing"

// TestCommandsRegistered guards against losing subcommands when files move
// (version once lived in the removed upstream update.go).
func TestCommandsRegistered(t *testing.T) {
	for _, name := range []string{"start", "stop", "restart", "status", "update", "version", "service", "login", "send"} {
		if c, _, err := rootCmd.Find([]string{name}); err != nil || c == rootCmd {
			t.Errorf("subcommand %q not registered", name)
		}
	}
	for _, name := range []string{"install", "uninstall"} {
		if c, _, err := rootCmd.Find([]string{"service", name}); err != nil || c.Name() != name {
			t.Errorf("subcommand %q not registered", "service "+name)
		}
	}
}
