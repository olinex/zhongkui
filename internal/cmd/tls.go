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
	Run: func(cmd *cobra.Command, args []string) {
		flags := cmd.Flags()
		name, err := flags.GetString("name")
		if err != nil {
			panic(fmt.Sprintf("invalid name: %v", err))
		}
		dirPath, err := flags.GetString("dir-path")
		if err != nil {
			panic(fmt.Sprintf("invalid path: %v", err))
		}
		ttl, err := flags.GetDuration("ttl")
		if err != nil {
			panic(fmt.Sprintf("invalid ttl: %v", err))
		}
		dnsNames, err := flags.GetStringArray("dns-names")
		if err != nil {
			panic(fmt.Sprintf("invalid dns names: %v", err))
		}
		inputs := TLSInputs{
			Name:     name,
			DirPath:  dirPath,
			TTL:      ttl,
			DNSNames: dnsNames,
		}
		// Validator is already initialized in PersistentPreRunE
		_, injector := cmdToolBox(cmd)
		validate := provider.MustInvokeValidate(injector)
		if err := validate.Struct(&inputs); err != nil {
			panic(err)
		}
		if err := os.MkdirAll(dirPath, os.ModeDir|os.ModePerm); err != nil {
			panic(err)
		}
		now := time.Now()
		key, cert, err := tlsutil.GenerateSelfSignedCert(now, now.Add(ttl), dnsNames)
		if err != nil {
			panic(fmt.Sprintf("cannot generate self cert: %v", err))
		}
		keyPath, certPath := path.Join(dirPath, fmt.Sprintf("%s-key.pem", name)), path.Join(dirPath, fmt.Sprintf("%s-cert.pem", name))
		keyFile, err := os.Create(keyPath)
		if err != nil {
			panic(err)
		}
		defer keyFile.Close()
		certFile, err := os.Create(certPath)
		if err != nil {
			panic(err)
		}
		defer certFile.Close()
		_, err = keyFile.WriteString(key)
		if err != nil {
			panic(err)
		}
		pterm.Success.Printf("tls key file %s was created\n", keyPath)
		_, err = certFile.WriteString(cert)
		if err != nil {
			panic(err)
		}
		pterm.Success.Printf("tls cert file %s was created\n", certPath)
		os.Exit(0)
	},
}

func init() {
	flags := generateTLSCmd.Flags()
	flags.String("name", "test", "name of the tls files")
	flags.String("dir-path", "./certs", "dir path to store tls files")
	flags.Duration("ttl", 24*365*time.Hour, "ttl for tls files from now")
	flags.StringArray("dns-names", []string{"zhongkui-security.com"}, "the domain names of the tls files")
}
