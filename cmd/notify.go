package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewNotifyCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "notify <message>",
		Short: "Print a placeholder notification message",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), args[0])
		},
	}
}
