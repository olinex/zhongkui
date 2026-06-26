package cmd

import (
	"context"
	"time"

	"github.com/gin-gonic/gin/binding"
	v "github.com/go-playground/validator/v10"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
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
	Use:   "create-user",
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
		gormDB := db.WithContext(ctx)
		errCh := make(chan error, 1)
		go func() {
			errCh <- user.Create(gormDB, superuser)
		}()
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			cancel()
			pterm.Error.Println("create user timeout")
			return ctx.Err()
		}
		cancel()
		if err := <-errCh; err != nil {
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
		gormDB := db.WithContext(ctx)
		errCh := make(chan error, 1)
		go func() {
			var user core.User
			if err := gormDB.Take(&user, &core.User{Username: username}).Error; err != nil {
				errCh <- err
				return
			}
			errCh <- user.ResetPassword(gormDB, password)
		}()
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			cancel()
			pterm.Error.Println("force reset password timeout")
			return ctx.Err()
		}
		cancel()
		if err := <-errCh; err != nil {
			return err
		}
		pterm.Success.Printf("reset user %s's password success\n", username)
		return nil
	},
}

func init() {
	flags := createUserCmd.Flags()
	flags.Duration("timeout", time.Minute, "timeout for inserting data into database")
	flags.Bool("superuser", false, "create as superuser")

	flags = forceResetPasswordCmd.Flags()
	flags.Duration("timeout", time.Minute, "timeout for inserting data into database")
}
