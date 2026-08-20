package cmd

import (
	"crypto/rand"
	"fmt"
	"os"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
	"zhongkui.com/zhongkui/internal/pkg/translate"
)

var passwordBookOutput string

var createPasswordBookCmd = &cobra.Command{
	Use:   "password-book",
	Short: "cmd.create.password-book.short",
	Long:  "cmd.create.password-book.long",
	Run: func(cmd *cobra.Command, args []string) {
		// Generate PasswordBookSize (65536) random bytes using crypto/rand
		data := make([]byte, key.PasswordBookSize)
		if _, err := rand.Read(data); err != nil {
			pterm.Error.Println(err.Error())
			return
		}

		if passwordBookOutput != "" {
			// Write to file
			if err := os.WriteFile(passwordBookOutput, data, 0600); err != nil {
				pterm.Error.Println(err.Error())
				return
			}
			fmt.Println(translate.T("cli.crypto.book-generated-file", map[string]any{"Path": passwordBookOutput, "Bytes": len(data)}))
		} else {
			// Write to stdout
			if _, err := os.Stdout.Write(data); err != nil {
				pterm.Error.Println(err.Error())
				return
			}
			fmt.Fprintln(os.Stderr, translate.T("cli.crypto.book-generated-stdout", map[string]any{"Bytes": len(data)}))
		}
	},
}

func init() {
	createPasswordBookCmd.Flags().StringVarP(&passwordBookOutput, "output", "o", "", "flag.create.password-book.output")
}
