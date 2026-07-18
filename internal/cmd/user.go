package cmd

import (
	"context"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/manager/routes"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/cliinput"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/provider"
)

func extractUserInput(v *viper.Viper) (routes.UserRegisterIn, error) {
	var in routes.UserRegisterIn
	if err := cliinput.Extract(v, &in, ""); err != nil {
		return in, err
	}
	return in, nil
}

func extractUsernamePassword(v *viper.Viper) (username, password string, err error) {
	var in routes.UserPasswordLoginIn
	if err := cliinput.Extract(v, &in, ""); err != nil {
		return "", "", err
	}
	return in.Username, in.Password, nil
}

var createUserCmd = &cobra.Command{
	Use:   "user",
	Short: "Create user by command line",
	Long:  "Create a new user. Parameters can be provided via command-line flags. Any missing parameters will be prompted interactively.",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, i := cmdToolBox(cmd)
		_ = provider.MustInvokeKeyDerivation(i)
		v := provider.MustInvokeViper(i)
		timeout := v.GetDuration("timeout")
		superuser := v.GetBool("superuser")

		userIn, err := extractUserInput(v)
		if err != nil {
			pterm.Error.Println(err.Error())
			return err
		}

		hashedPassword, err := crypto.HashPassword(userIn.Password)
		if err != nil {
			return err
		}

		db := provider.MustInvokeCoreDatabase(i)
		user := &core.User{
			UnionID:    serializer.NewHexUUIDv4(),
			Username:   userIn.Username,
			NickleName: userIn.NickleName,
			Password:   serializer.NewCrypto(hashedPassword),
			Email:      userIn.Email,
			Active:     true,
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		if err := user.Create(db.WithContext(ctx), superuser); err != nil {
			pterm.Error.Println(err.Error())
			return err
		}

		pterm.Success.Printf("create user %s success\n", user.Username)
		return nil
	},
}

var forceResetPasswordCmd = &cobra.Command{
	Use:   "force-reset-password",
	Short: "Force reset user's password by command line",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, i := cmdToolBox(cmd)
		_ = provider.MustInvokeKeyDerivation(i)
		v := provider.MustInvokeViper(i)
		timeout := v.GetDuration("timeout")

		db := provider.MustInvokeCoreDatabase(i)
		provider.MustInvokeRbac(i)
		username, password, err := extractUsernamePassword(v)
		if err != nil {
			pterm.Error.Println(err.Error())
			return err
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		var user core.User
		if err := db.WithContext(ctx).Take(&user, &core.User{Username: username}).Error; err != nil {
			pterm.Error.Println(err.Error())
			return err
		}

		if err := user.ResetPassword(db.WithContext(ctx), password); err != nil {
			pterm.Error.Println(err.Error())
			return err
		}

		pterm.Success.Printf("reset user %s's password success\n", username)
		return nil
	},
}

type createAccessKeyInput struct {
	Username    string        `viper:"username" prompt:"please enter username" binding:"required,min=6,max=32,alphaunicode"`
	Description string        `viper:"description" prompt:"please enter description" binding:"required,min=6,max=256"`
	Enabled     bool          `viper:"enabled" prompt:"enable this access key?"`
	TTL         time.Duration `viper:"ttl" prompt:"please enter ttl (e.g. 8760h, 0 for never)"`
}

var createAccessKeyCmd = &cobra.Command{
	Use:   "access-key",
	Short: "Create access key for a user",
	Long:  "Create an API access key for an existing user. Parameters can be provided via command-line flags or prompted interactively.",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, i := cmdToolBox(cmd)
		_ = provider.MustInvokeKeyDerivation(i)
		v := provider.MustInvokeViper(i)
		timeout := v.GetDuration("timeout")

		var in createAccessKeyInput
		if err := cliinput.Extract(v, &in, ""); err != nil {
			pterm.Error.Println(err.Error())
			return err
		}

		// 从配置文件读取最大数量，通过 ExtractConfig 获取带默认值的配置
		cfg, err := manager.ExtractConfig(v)
		if err != nil {
			pterm.Error.Printf("cannot extract config: %v\n", err)
			return err
		}
		maxCount := int(cfg.Auth.AccessKeyCount)

		db := provider.MustInvokeCoreDatabase(i)

		// 查找用户
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		var user core.User
		if err := db.WithContext(ctx).Take(&user, &core.User{Username: in.Username}).Error; err != nil {
			pterm.Error.Printf("cannot find user %s: %v\n", in.Username, err)
			return err
		}

		// 计算过期时间
		var expiredAt *time.Time
		if in.TTL > 0 {
			expired := time.Now().Add(in.TTL)
			expiredAt = &expired
		}

		// 创建访问密钥对象
		key, err := core.NewAccessKey(&user, in.Enabled, in.Description, expiredAt)
		if err != nil {
			pterm.Error.Printf("cannot create access key: %v\n", err)
			return err
		}

		// 使用复用方法在事务中创建
		if err := key.CreateWithLimit(db.WithContext(ctx), maxCount); err != nil {
			if err == core.ErrTooManyAccessKey {
				pterm.Error.Printf("user %s already has %d access keys (max: %d)\n", in.Username, maxCount, maxCount)
			} else {
				pterm.Error.Printf("cannot create access key: %v\n", err)
			}
			return err
		}

		// 输出访问密钥信息
		pterm.Success.Printf("access key created for user %s\n", in.Username)
		pterm.Info.Printf("  AccessKeyID:     %s\n", key.AccessKeyID)
		pterm.Info.Printf("  AccessKeySecret: %s\n", key.Secret())
		pterm.Info.Printf("  Description:     %s\n", key.Description)
		pterm.Info.Printf("  Enabled:         %v\n", key.Enabled)
		if key.RawExpiredAt != nil {
			pterm.Info.Printf("  ExpiredAt:       %s\n", key.RawExpiredAt.Format(time.RFC3339))
		} else {
			pterm.Info.Printf("  ExpiredAt:       never\n")
		}
		return nil
	},
}

func init() {
	flags := createUserCmd.Flags()
	flags.StringP("username", "u", "", "username for the new user (min 6 chars)")
	flags.StringP("password", "p", "", "password for the new user (min 8 chars)")
	flags.StringP("nicklename", "n", "", "nicklename for the new user (min 6 chars)")
	flags.StringP("email", "e", "", "email for the new user")
	flags.Duration("timeout", time.Minute, "timeout for inserting data into database")
	flags.Bool("superuser", false, "create as superuser")

	flags = forceResetPasswordCmd.Flags()
	flags.StringP("username", "u", "", "username of the user to reset password for")
	flags.StringP("password", "p", "", "new password for the user")
	flags.Duration("timeout", time.Minute, "timeout for inserting data into database")

	flags = createAccessKeyCmd.Flags()
	flags.StringP("username", "u", "", "username of the user to create access key for")
	flags.StringP("description", "d", "", "description of the access key")
	flags.Bool("enabled", true, "whether the access key is enabled")
	flags.Duration("ttl", 0, "time-to-live for the access key from now (0 means never expires)")
	flags.Duration("timeout", time.Minute, "timeout for database operations")
}
