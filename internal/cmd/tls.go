package cmd

import (
	"fmt"
	"os"
	"path"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"zhongkuisec.com/zhongkui/internal/pkg/cliinput"
	tlsutil "zhongkuisec.com/zhongkui/internal/pkg/tls"
	"zhongkuisec.com/zhongkui/internal/pkg/translate"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
	"zhongkuisec.com/zhongkui/internal/provider"
)

// TLSInputs tls 子命令的输入参数。
type TLSInputs struct {
	// Name TLS 名称，用于生成密钥与证书文件名。
	Name string `viper:"name" prompt:"请输入 TLS 名称" binding:"required,min=2,max=255,strictFileName"`
	// DirPath 密钥与证书的输出目录。
	DirPath string `viper:"dir-path" prompt:"请输入目录路径" binding:"required,min=2,max=1024,dirpath"`
	// TTL 证书有效期。
	TTL time.Duration `viper:"ttl" prompt:"请输入有效期 TTL（例如 8760h 表示 1 年）" binding:"required"`
	// DNSNames 证书包含的 DNS 域名列表。
	DNSNames []string `viper:"dns-names" prompt:"请输入 DNS 域名列表" binding:"required,min=1"`
}

var generateTLSCmd = &cobra.Command{
	Use:   "tls",
	Short: "cmd.create.tls.short",
	Long:  "cmd.create.tls.long",
	Run: func(cmd *cobra.Command, args []string) {
		_, injector := cmdToolBox(cmd)
		v := provider.MustInvokeViper(injector)

		var inputs TLSInputs
		if err := cliinput.Extract(v, validator.Validate, &inputs, ""); err != nil {
			pterm.Error.Println(err.Error())
			return
		}

		if err := os.MkdirAll(inputs.DirPath, os.ModeDir|os.ModePerm); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		now := time.Now()
		key, cert, err := tlsutil.GenerateSelfSignedCert(now, now.Add(inputs.TTL), inputs.DNSNames)
		if err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		keyPath, certPath := path.Join(inputs.DirPath, fmt.Sprintf("%s-key.pem", inputs.Name)), path.Join(inputs.DirPath, fmt.Sprintf("%s-cert.pem", inputs.Name))
		keyFile, err := os.Create(keyPath)
		if err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		defer keyFile.Close()
		certFile, err := os.Create(certPath)
		if err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		defer certFile.Close()
		if _, err = keyFile.WriteString(key); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		pterm.Success.Println(translate.T("cli.tls.key-created", map[string]any{"Path": keyPath}))
		if _, err = certFile.WriteString(cert); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		pterm.Success.Println(translate.T("cli.tls.cert-created", map[string]any{"Path": certPath}))
	},
}

func init() {
	flags := generateTLSCmd.Flags()
	flags.String("name", "", "flag.create.tls.name")
	flags.String("dir-path", "", "flag.create.tls.dir-path")
	flags.Duration("ttl", 0, "flag.create.tls.ttl")
	flags.StringArray("dns-names", nil, "flag.create.tls.dns-names")
}
