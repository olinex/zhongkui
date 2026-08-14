package cmd

import (
	"crypto/rand"
	"fmt"
	"os"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
)

var passwordBookOutput string

var createPasswordBookCmd = &cobra.Command{
	Use:   "password-book",
	Short: "生成用于密钥派生的密码本文件",
	Long:  "生成一个加密安全的随机密码本文件（65536 字节），用于子密钥派生。",
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
			fmt.Println(cliT("cli.crypto.book-generated-file", "密码本文件已生成：{{.Path}}（{{.Bytes}} 字节）", map[string]any{"Path": passwordBookOutput, "Bytes": len(data)}))
		} else {
			// Write to stdout
			if _, err := os.Stdout.Write(data); err != nil {
				pterm.Error.Println(err.Error())
				return
			}
			fmt.Fprintln(os.Stderr, cliT("cli.crypto.book-generated-stdout", "密码本已生成（{{.Bytes}} 字节）。使用 --output 保存到文件。", map[string]any{"Bytes": len(data)}))
		}
	},
}

func init() {
	createPasswordBookCmd.Flags().StringVarP(&passwordBookOutput, "output", "o", "", "输出文件路径（未指定时输出到标准输出）")
}
