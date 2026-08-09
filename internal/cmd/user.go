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

// createUserInput 定义命令行创建用户的输入结构体
// 继承 HTTP 注册输入，并额外包含是否为超级管理员的交互式确认
type createUserInput struct {
	routes.UserRegisterIn
	Superuser bool `viper:"superuser" prompt:"是否为超级管理员？"`
}

func extractUserInput(v *viper.Viper) (createUserInput, error) {
	var in createUserInput
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
	Short: "通过命令行创建用户",
	Long:  "创建一个新用户。参数可以通过命令行标志提供，缺失的参数将交互式提示输入。",
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

		pterm.Success.Printf("create user %s success\n", user.Username)
	},
}

var forceResetPasswordCmd = &cobra.Command{
	Use:   "force-reset-password",
	Short: "通过命令行强制重置用户密码",
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

		pterm.Success.Printf("reset user %s's password success\n", username)
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
	Short: "为用户创建访问密钥",
	Long:  "为已有用户创建 API 访问密钥。参数可以通过命令行标志提供，也可以通过交互式提示输入。",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
		_ = provider.MustInvokeKeyDerivation(i)
		v := provider.MustInvokeViper(i)
		timeout := v.GetDuration("timeout")

		var in createAccessKeyInput
		if err := cliinput.Extract(v, &in, ""); err != nil {
			pterm.Error.Println(err.Error())
			return
		}

		// 从配置文件读取最大数量，通过 ExtractConfig 获取带默认值的配置
		cfg, err := manager.ExtractConfig(v)
		if err != nil {
			pterm.Error.Printf("cannot extract config: %v\n", err)
			return
		}
		maxCount := int(cfg.Auth.AccessKeyCount)

		db := provider.MustInvokeCoreDatabase(i)

		// 查找用户
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		var user core.User
		if err := db.WithContext(ctx).Take(&user, &core.User{Username: in.Username}).Error; err != nil {
			pterm.Error.Printf("cannot find user %s: %v\n", in.Username, err)
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
			pterm.Error.Printf("cannot create access key: %v\n", err)
			return
		}

		// 使用复用方法在事务中创建
		if err := key.CreateWithLimit(db.WithContext(ctx), maxCount); err != nil {
			if err == core.ErrTooManyAccessKey {
				pterm.Error.Printf("user %s already has %d access keys (max: %d)\n", in.Username, maxCount, maxCount)
			} else {
				pterm.Error.Printf("cannot create access key: %v\n", err)
			}
			return
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
	},
}

func init() {
	flags := createUserCmd.Flags()
	flags.StringP("username", "u", "", "新用户的用户名（至少 6 个字符）")
	flags.StringP("password", "p", "", "新用户的密码（至少 8 个字符）")
	flags.StringP("nicklename", "n", "", "新用户的昵称（至少 6 个字符）")
	flags.StringP("email", "e", "", "新用户的邮箱")
	flags.Duration("timeout", time.Minute, "向数据库写入数据的超时时间")

	flags = forceResetPasswordCmd.Flags()
	flags.StringP("username", "u", "", "要重置密码的用户名")
	flags.StringP("password", "p", "", "用户的新密码")
	flags.Duration("timeout", time.Minute, "向数据库写入数据的超时时间")

	flags = createAccessKeyCmd.Flags()
	flags.StringP("username", "u", "", "要创建访问密钥的用户名")
	flags.StringP("description", "d", "", "访问密钥的描述")
	flags.Bool("enabled", true, "访问密钥是否启用")
	flags.Duration("ttl", 0, "访问密钥自当前时刻起的有效期（0 表示永不过期）")
	flags.Duration("timeout", time.Minute, "数据库操作超时时间")
}
