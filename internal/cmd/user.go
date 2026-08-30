package cmd

import (
	"context"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/manager/views"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/cliinput"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/translate"
	"zhongkui.com/zhongkui/internal/pkg/validator"
	"zhongkui.com/zhongkui/internal/provider"
)

// createUserInput 定义命令行创建用户的输入结构体
// 继承 HTTP 注册输入，并额外包含是否为超级管理员的交互式确认
type createUserInput struct {
	views.UserRegisterIn
	Superuser bool `viper:"superuser" prompt:"是否为超级管理员？"`
}

func extractUserInput(v *viper.Viper) (createUserInput, error) {
	var in createUserInput
	if err := cliinput.Extract(v, validator.Validate, &in, ""); err != nil {
		return in, err
	}
	return in, nil
}

func extractUsernamePassword(v *viper.Viper) (username, password string, err error) {
	var in views.UserPasswordLoginIn
	if err := cliinput.Extract(v, validator.Validate, &in, ""); err != nil {
		return "", "", err
	}
	return in.Username, in.Password, nil
}

var createUserCmd = &cobra.Command{
	Use:   "user",
	Short: "cmd.create.user.short",
	Long:  "cmd.create.user.long",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
		_ = provider.MustInvokeKeyDerivation(i)
		v := provider.MustInvokeViper(i)
		timeout := v.GetDuration("timeout")

		userIn, err := extractUserInput(v)
		if err != nil {
			pterm.Error.Println(err.Error())
			return
		}

		hashedPassword, err := crypto.HashPassword(userIn.Password)
		if err != nil {
			pterm.Error.Println(err.Error())
			return
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

		if err := user.Create(db.WithContext(ctx), userIn.Superuser); err != nil {
			pterm.Error.Println(err.Error())
			return
		}

		pterm.Success.Println(translate.T("cli.user.create-success", map[string]any{"Username": user.Username}))
	},
}

var forceResetPasswordCmd = &cobra.Command{
	Use:   "force-reset-password",
	Short: "cmd.force-reset-password.short",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
		_ = provider.MustInvokeKeyDerivation(i)
		v := provider.MustInvokeViper(i)
		timeout := v.GetDuration("timeout")

		db := provider.MustInvokeCoreDatabase(i)
		provider.MustInvokeRbac(i)
		username, password, err := extractUsernamePassword(v)
		if err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		var user core.User
		if err := db.WithContext(ctx).Take(&user, &core.User{Username: username}).Error; err != nil {
			pterm.Error.Println(err.Error())
			return
		}

		if err := user.ResetPassword(db.WithContext(ctx), password); err != nil {
			pterm.Error.Println(err.Error())
			return
		}

		pterm.Success.Println(translate.T("cli.user.reset-password-success", map[string]any{"Username": username}))
	},
}

type createAccessKeyInput struct {
	Username    string        `viper:"username" prompt:"请输入用户名" binding:"required,min=6,max=32,alphaunicode"`
	Description string        `viper:"description" prompt:"请输入描述" binding:"required,min=6,max=256"`
	Enabled     bool          `viper:"enabled" prompt:"是否启用该访问密钥？"`
	TTL         time.Duration `viper:"ttl" prompt:"请输入有效期 TTL（如 8760h，0 表示永不过期）"`
}

var createAccessKeyCmd = &cobra.Command{
	Use:   "access-key",
	Short: "cmd.create.access-key.short",
	Long:  "cmd.create.access-key.long",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
		_ = provider.MustInvokeKeyDerivation(i)
		v := provider.MustInvokeViper(i)
		timeout := v.GetDuration("timeout")

		var in createAccessKeyInput
		if err := cliinput.Extract(v, validator.Validate, &in, ""); err != nil {
			pterm.Error.Println(err.Error())
			return
		}

		// 从配置文件读取最大数量，通过 ExtractConfig 获取带默认值的配置
		cfg, err := manager.ExtractConfig(v, validator.Validate)
		if err != nil {
			pterm.Error.Println(translate.T("cli.user.extract-config-failed", map[string]any{"Err": err}))
			return
		}
		maxCount := int(cfg.Auth.AccessKeyCount)

		db := provider.MustInvokeCoreDatabase(i)

		// 查找用户
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		var user core.User
		if err := db.WithContext(ctx).Take(&user, &core.User{Username: in.Username}).Error; err != nil {
			pterm.Error.Println(translate.T("cli.user.not-found", map[string]any{"Username": in.Username, "Err": err}))
			return
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
			pterm.Error.Println(translate.T("cli.user.access-key-create-failed", map[string]any{"Err": err}))
			return
		}

		// 使用复用方法在事务中创建
		if err := key.CreateWithLimit(db.WithContext(ctx), maxCount); err != nil {
			if err == core.ErrTooManyAccessKey {
				pterm.Error.Println(translate.T("cli.user.access-key-max", map[string]any{"Username": in.Username, "Max": maxCount}))
			} else {
				pterm.Error.Println(translate.T("cli.user.access-key-create-failed", map[string]any{"Err": err}))
			}
			return
		}

		// 输出访问密钥信息
		pterm.Success.Println(translate.T("cli.user.access-key-created", map[string]any{"Username": in.Username}))
		pterm.Info.Println(translate.T("cli.user.access-key-id", map[string]any{"Value": key.AccessKeyID}))
		pterm.Info.Println(translate.T("cli.user.access-key-secret", map[string]any{"Value": key.Secret()}))
		pterm.Info.Println(translate.T("cli.user.access-key-description", map[string]any{"Value": key.Description}))
		pterm.Info.Println(translate.T("cli.user.access-key-enabled", map[string]any{"Value": key.Enabled}))
		if key.RawExpiredAt != nil {
			pterm.Info.Println(translate.T("cli.user.access-key-expired-at", map[string]any{"Value": key.RawExpiredAt.Format(time.RFC3339)}))
		} else {
			pterm.Info.Println(translate.T("cli.user.access-key-never-expires", nil))
		}
	},
}

func init() {
	flags := createUserCmd.Flags()
	flags.StringP("username", "u", "", "flag.create.user.username")
	flags.StringP("password", "p", "", "flag.create.user.password")
	flags.StringP("nicklename", "n", "", "flag.create.user.nicklename")
	flags.StringP("email", "e", "", "flag.create.user.email")
	flags.Duration("timeout", time.Minute, "flag.create.user.timeout")

	flags = forceResetPasswordCmd.Flags()
	flags.StringP("username", "u", "", "flag.force-reset-password.username")
	flags.StringP("password", "p", "", "flag.force-reset-password.password")
	flags.Duration("timeout", time.Minute, "flag.force-reset-password.timeout")

	flags = createAccessKeyCmd.Flags()
	flags.StringP("username", "u", "", "flag.create.access-key.username")
	flags.StringP("description", "d", "", "flag.create.access-key.description")
	flags.Bool("enabled", true, "flag.create.access-key.enabled")
	flags.Duration("ttl", 0, "flag.create.access-key.ttl")
	flags.Duration("timeout", time.Minute, "flag.create.access-key.timeout")
}
