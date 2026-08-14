package cmd

import (
	"fmt"
	"os"
	"path"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"zhongkui.com/zhongkui/internal/pkg/cliinput"
	tlsutil "zhongkui.com/zhongkui/internal/pkg/tls"
	"zhongkui.com/zhongkui/internal/pkg/validator"
	"zhongkui.com/zhongkui/internal/provider"
)

type TLSInputs struct {
	Name     string        `viper:"name" prompt:"请输入 TLS 名称" binding:"required,min=2,max=255,strictFileName"`
	DirPath  string        `viper:"dir-path" prompt:"请输入目录路径" binding:"required,min=2,max=1024,dirpath"`
	TTL      time.Duration `viper:"ttl" prompt:"请输入有效期 TTL（例如 8760h 表示 1 年）" binding:"required"`
	DNSNames []string      `viper:"dns-names" prompt:"请输入 DNS 域名列表" binding:"required,min=1"`
}

var generateTLSCmd = &cobra.Command{
	Use:   "tls",
	Short: "创建自签名 TLS 证书",
	Long:  "生成自签名 TLS 证书和密钥对。参数可以通过命令行标志提供，也可以通过交互式提示输入。",
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
		pterm.Success.Println(cliT("cli.tls.key-created", "TLS 密钥文件 {{.Path}} 已创建", map[string]any{"Path": keyPath}))
		if _, err = certFile.WriteString(cert); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		pterm.Success.Println(cliT("cli.tls.cert-created", "TLS 证书文件 {{.Path}} 已创建", map[string]any{"Path": certPath}))
	},
}

func init() {
	flags := generateTLSCmd.Flags()
	flags.String("name", "", "TLS 文件名称")
	flags.String("dir-path", "", "TLS 文件存储目录路径")
	flags.Duration("ttl", 0, "TLS 文件自当前时刻起的有效期（如 8760h）")
	flags.StringArray("dns-names", nil, "TLS 文件的域名列表（逗号分隔）")
}
