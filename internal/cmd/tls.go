package cmd

import (
	"fmt"
	"os"
	"path"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"zhongkui.com/zhongkui/internal/provider"
	tlsutil "zhongkui.com/zhongkui/internal/pkg/tls"
)

type TLSInputs struct {
	Name     string        `binding:"required,min=2,max=255,strictFileName"`
	DirPath  string        `binding:"required,min=2,max=1024,dirpath"`
	TTL      time.Duration `binding:"required"`
	DNSNames []string      `binding:"required,min=1"`
}

var generateTLSCmd = &cobra.Command{
	Use:   "create-tls",
	Short: "Create tls by self Signed",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, injector := cmdToolBox(cmd)
		v := provider.MustInvokeViper(injector)
		name := v.GetString("name")
		dirPath := v.GetString("dir-path")
		ttl := v.GetDuration("ttl")
		dnsNames := v.GetStringSlice("dns-names")

		inputs := TLSInputs{
			Name:     name,
			DirPath:  dirPath,
			TTL:      ttl,
			DNSNames: dnsNames,
		}
		validate := provider.MustInvokeValidate(injector)
		if err := validate.Struct(&inputs); err != nil {
			return err
		}
		if err := os.MkdirAll(dirPath, os.ModeDir|os.ModePerm); err != nil {
			return err
		}
		now := time.Now()
		key, cert, err := tlsutil.GenerateSelfSignedCert(now, now.Add(ttl), dnsNames)
		if err != nil {
			return fmt.Errorf("cannot generate self cert: %v", err)
		}
		keyPath, certPath := path.Join(dirPath, fmt.Sprintf("%s-key.pem", name)), path.Join(dirPath, fmt.Sprintf("%s-cert.pem", name))
		keyFile, err := os.Create(keyPath)
		if err != nil {
			return err
		}
		defer keyFile.Close()
		certFile, err := os.Create(certPath)
		if err != nil {
			return err
		}
		defer certFile.Close()
		if _, err = keyFile.WriteString(key); err != nil {
			return err
		}
		pterm.Success.Printf("tls key file %s was created\n", keyPath)
		if _, err = certFile.WriteString(cert); err != nil {
			return err
		}
		pterm.Success.Printf("tls cert file %s was created\n", certPath)
		return nil
	},
}

func init() {
	flags := generateTLSCmd.Flags()
	flags.String("name", "test", "name of the tls files")
	flags.String("dir-path", "./certs", "dir path to store tls files")
	flags.Duration("ttl", 24*365*time.Hour, "ttl for tls files from now")
	flags.StringArray("dns-names", []string{"zhongkui-security.com"}, "the domain names of the tls files")
}
