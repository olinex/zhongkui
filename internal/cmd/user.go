package cmd

import (
	"context"
	"time"

	"github.com/gin-gonic/gin/binding"
	v "github.com/go-playground/validator/v10"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/manager/routes"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/validator"
	"zhongkui.com/zhongkui/internal/provider"
)

func extractUser() (*core.User, error) {
	var err error
	userIn := routes.UserRegisterIn{}
	validator := binding.Validator.Engine().(*v.Validate)
	validator.SetTagName("binding")
	username, password, err := extractUsernamePassword()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	userIn.Username = username
	userIn.Password = password
	// nicklename
	userIn.NickleName, err = pterm.DefaultInteractiveTextInput.Show("please enter nicklename")
	if err != nil {
		return nil, err
	}
	err = validator.StructPartial(&userIn, "UserProfileChangeIn.NickleName")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// email
	userIn.Email, err = pterm.DefaultInteractiveTextInput.Show("please enter user's email")
	if err != nil {
		return nil, err
	}
	err = validator.StructPartial(&userIn, "UserProfileChangeIn.Email")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	hashedPassword, err := crypto.HashPassword(userIn.Password)
	if err != nil {
		return nil, err
	}
	return &core.User{
		UnionID:    serializer.NewHexUUIDv4(),
		Username:   userIn.Username,
		NickleName: userIn.NickleName,
		Password:   hashedPassword,
		Email:      userIn.Email,
		Active:     true,
	}, nil
}

func extractUsernamePassword() (username, password string, err error) {
	userIn := routes.UserRegisterIn{}
	// username
	userIn.Username, err = pterm.DefaultInteractiveTextInput.Show("please enter username")
	if err != nil {
		return "", "", err
	}
	err = validator.Validate.StructPartial(&userIn, "UserPasswordLoginIn.Username")
	if err != nil {
		return "", "", errors.Newf("Bad username cause: %v", err)
	}
	// password
	for {
		userIn.Password, err = pterm.DefaultInteractiveTextInput.WithMask("*").Show("please enter password")
		if err != nil {
			return "", "", err
		}
		err = validator.Validate.StructPartial(&userIn, "UserPasswordLoginIn.Password")
		if err != nil {
			return "", "", errors.Newf("Bad password cause: %v", err)
		}
		passwordAgain, err := pterm.DefaultInteractiveTextInput.WithMask("*").Show("please enter password again")
		if err != nil {
			return "", "", err
		}
		err = validator.Validate.StructPartial(&userIn, "UserPasswordLoginIn.PasswordAgain")
		if err != nil {
			return "", "", errors.Newf("Bad password again cause: %v", err)
		}
		if userIn.Password != passwordAgain {
			pterm.Warning.Println("password double check failed, please try again!")
			continue
		}
		break
	}
	return userIn.Username, userIn.Password, nil
}

var createUserCmd = &cobra.Command{
	Use:   "user",
	Short: "Create user by command line",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, injector := cmdToolBox(cmd)
		v := provider.MustInvokeViper(injector)
		timeout := v.GetDuration("timeout")
		superuser := v.GetBool("superuser")

		db := provider.MustInvokeCoreDatabase(injector)
		user, err := extractUser()
		if err != nil {
			pterm.Error.Println(err.Error())
			return err
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
		ctx, injector := cmdToolBox(cmd)
		v := provider.MustInvokeViper(injector)
		timeout := v.GetDuration("timeout")

		db := provider.MustInvokeCoreDatabase(injector)
		provider.MustInvokeRbac(injector)
		username, password, err := extractUsernamePassword()
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

var createAccessKeyCmd = &cobra.Command{
	Use:   "access-key",
	Short: "Create access key for a user",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, injector := cmdToolBox(cmd)
		v := provider.MustInvokeViper(injector)
		username := v.GetString("username")
		description := v.GetString("description")
		enabled := v.GetBool("enabled")
		ttl := v.GetDuration("ttl")
		timeout := v.GetDuration("timeout")
		// 从配置文件读取最大数量，通过 ExtractConfig 获取带默认值的配置
		cfg, err := manager.ExtractConfig(v)
		if err != nil {
			pterm.Error.Printf("cannot extract config: %v\n", err)
			return err
		}
		maxCount := int(cfg.Auth.AccessKeyCount)

		db := provider.MustInvokeCoreDatabase(injector)

		// 查找用户
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		var user core.User
		if err := db.WithContext(ctx).Take(&user, &core.User{Username: username}).Error; err != nil {
			pterm.Error.Printf("cannot find user %s: %v\n", username, err)
			return err
		}

		// 计算过期时间
		var expiredAt *time.Time
		if ttl > 0 {
			expired := time.Now().Add(ttl)
			expiredAt = &expired
		}

		// 创建访问密钥对象
		key, err := core.NewAccessKey(&user, enabled, description, expiredAt)
		if err != nil {
			pterm.Error.Printf("cannot create access key: %v\n", err)
			return err
		}

		// 使用复用方法在事务中创建
		if err := key.CreateWithLimit(db.WithContext(ctx), maxCount); err != nil {
			if err == core.ErrTooManyAccessKey {
				pterm.Error.Printf("user %s already has %d access keys (max: %d)\n", username, maxCount, maxCount)
			} else {
				pterm.Error.Printf("cannot create access key: %v\n", err)
			}
			return err
		}

		// 输出访问密钥信息
		pterm.Success.Printf("access key created for user %s\n", username)
		pterm.Info.Printf("  AccessKeyID:     %s\n", key.AccessKeyID)
		pterm.Info.Printf("  AccessKeySecret: %s\n", key.AccessKeySecret)
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
	flags.Duration("timeout", time.Minute, "timeout for inserting data into database")
	flags.Bool("superuser", false, "create as superuser")

	flags = forceResetPasswordCmd.Flags()
	flags.Duration("timeout", time.Minute, "timeout for inserting data into database")

	flags = createAccessKeyCmd.Flags()
	flags.StringP("username", "u", "", "username of the user to create access key for (required)")
	flags.StringP("description", "d", "Created by CLI", "description of the access key")
	flags.Bool("enabled", true, "whether the access key is enabled")
	flags.Duration("ttl", 0, "time-to-live for the access key from now (0 means never expires)")
	flags.Duration("timeout", time.Minute, "timeout for database operations")
	createAccessKeyCmd.MarkFlagRequired("username")
}
