package cmd

import (
	"crypto/rand"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

var passwordBookOutput string

var createPasswordBookCmd = &cobra.Command{
	Use:   "password-book",
	Short: "Generate a password book file for key derivation",
	Long:  "Generate a cryptographically secure random password book file (65536 bytes) used for child key derivation.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Generate PasswordBookSize (65536) random bytes using crypto/rand
		data := make([]byte, crypto.PASSWORD_BOOK_SIZE)
		if _, err := rand.Read(data); err != nil {
			return errors.WithStack(err)
		}

		if passwordBookOutput != "" {
			// Write to file
			if err := os.WriteFile(passwordBookOutput, data, 0600); err != nil {
				return errors.WithStack(err)
			}
			fmt.Printf("Password book generated successfully: %s (%d bytes)\n", passwordBookOutput, len(data))
		} else {
			// Write to stdout
			if _, err := os.Stdout.Write(data); err != nil {
				return errors.WithStack(err)
			}
			fmt.Fprintf(os.Stderr, "Password book generated successfully (%d bytes). Use --output to save to a file.\n", len(data))
		}
		return nil
	},
}

func init() {
	createPasswordBookCmd.Flags().StringVarP(&passwordBookOutput, "output", "o", "", "Output file path (if not specified, output to stdout)")
}
