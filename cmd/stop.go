package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(stopCmd)
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the weclaw bridge",
	RunE: func(cmd *cobra.Command, args []string) error {
		if serviceInstalled() {
			if err := serviceStop(); err != nil {
				return err
			}
			fmt.Println("weclaw service stopped (starts again at next login or 'weclaw start')")
			return nil
		}
		stopAllWeclaw()
		fmt.Println("weclaw stopped")
		return nil
	},
}
