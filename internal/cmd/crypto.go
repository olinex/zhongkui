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
		// 使用 crypto/rand 生成 PasswordBookSize（65536）字节的随机数据
		data := make([]byte, key.PasswordBookSize)
		if _, err := rand.Read(data); err != nil {
			pterm.Error.Println(err.Error())
			return
		}

		if passwordBookOutput != "" {
			// 写入文件
			if err := os.WriteFile(passwordBookOutput, data, 0600); err != nil {
				pterm.Error.Println(err.Error())
				return
			}
			fmt.Println(translate.T("cli.crypto.book-generated-file", map[string]any{"Path": passwordBookOutput, "Bytes": len(data)}))
		} else {
			// 写入标准输出
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
