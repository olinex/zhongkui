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
	"zhongkui.com/zhongkui/internal/provider"
)

type TLSInputs struct {
	Name     string        `viper:"name" prompt:"please enter tls name" binding:"required,min=2,max=255,strictFileName"`
	DirPath  string        `viper:"dir-path" prompt:"please enter directory path" binding:"required,min=2,max=1024,dirpath"`
	TTL      time.Duration `viper:"ttl" prompt:"please enter ttl (e.g. 8760h for 1 year)" binding:"required"`
	DNSNames []string      `viper:"dns-names" prompt:"please enter dns names" binding:"required,min=1"`
}

var generateTLSCmd = &cobra.Command{
	Use:   "tls",
	Short: "Create tls by self Signed",
	Long:  "Generate a self-signed TLS certificate and key pair. Parameters can be provided via command-line flags or prompted interactively.",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, injector := cmdToolBox(cmd)
		v := provider.MustInvokeViper(injector)

		var inputs TLSInputs
		if err := cliinput.Extract(v, &inputs, ""); err != nil {
			pterm.Error.Println(err.Error())
			return err
		}

		if err := os.MkdirAll(inputs.DirPath, os.ModeDir|os.ModePerm); err != nil {
			return err
		}
		now := time.Now()
		key, cert, err := tlsutil.GenerateSelfSignedCert(now, now.Add(inputs.TTL), inputs.DNSNames)
		if err != nil {
			return fmt.Errorf("cannot generate self cert: %v", err)
		}
		keyPath, certPath := path.Join(inputs.DirPath, fmt.Sprintf("%s-key.pem", inputs.Name)), path.Join(inputs.DirPath, fmt.Sprintf("%s-cert.pem", inputs.Name))
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
	flags.String("name", "", "name of the tls files")
	flags.String("dir-path", "", "dir path to store tls files")
	flags.Duration("ttl", 0, "ttl for tls files from now (e.g. 8760h)")
	flags.StringArray("dns-names", nil, "the domain names of the tls files (comma-separated)")
}
