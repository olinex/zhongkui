package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{
	Use:   "help",
	Short: "The zhongkui root command lines",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	RootCmd.AddCommand(runCmd)
	RootCmd.AddCommand(describeConfigCmd)
	RootCmd.AddCommand(migrateCmd)
	RootCmd.AddCommand(clearCmd)
	RootCmd.AddCommand(createUserCmd)
	RootCmd.AddCommand(generateTLSCmd)
	RootCmd.AddCommand(forceResetPasswordCmd)
}
