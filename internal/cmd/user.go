package cmd

import (
	"context"
	"fmt"
	"os"
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
		panic(err)
	}
	err = validator.StructPartial(&userIn, "UserProfileChangeIn.NickleName")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// email
	userIn.Email, err = pterm.DefaultInteractiveTextInput.Show("please enter user's email")
	if err != nil {
		panic(err)
	}
	err = validator.StructPartial(&userIn, "UserProfileChangeIn.Email")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	hashedPassword, err := crypto.HashPassword(userIn.Password)
	if err != nil {
		panic(err)
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
	validator := validator.MustValidate()
	// username
	userIn.Username, err = pterm.DefaultInteractiveTextInput.Show("please enter username")
	if err != nil {
		panic(err)
	}
	err = validator.StructPartial(&userIn, "UserPasswordLoginIn.Username")
	if err != nil {
		return "", "", errors.Newf("Bad username cause: %v", err)
	}
	// password
	for {
		userIn.Password, err = pterm.DefaultInteractiveTextInput.WithMask("*").Show("please enter password")
		if err != nil {
			panic(err)
		}
		err = validator.StructPartial(&userIn, "UserPasswordLoginIn.Password")
		if err != nil {
			return "", "", errors.Newf("Bad password cause: %v", err)
		}
		passwordAgain, err := pterm.DefaultInteractiveTextInput.WithMask("*").Show("please enter password again")
		if err != nil {
			panic(err)
		}
		err = validator.StructPartial(&userIn, "UserPasswordLoginIn.PasswordAgain")
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
	Run: func(cmd *cobra.Command, args []string) {
		flags := cmd.Flags()
		timeout, err := flags.GetDuration("timeout")
		if err != nil {
			panic(fmt.Sprintf("invalid timeout: %v", err))
		}
		superuser, err := flags.GetBool("superuser")
		if err != nil {
			panic(fmt.Sprintf("invalid superuser: %v", err))
		}
		_, injector := cmdToolBox(cmd)
		// Force validater initialization
		provider.MustInvokeValidate(injector)
		db := provider.MustInvokeCoreDatabase(injector)
		// Force rbac initialization
		provider.MustInvokeRbac(injector)
		user, err := extractUser()
		if err != nil {
			pterm.Error.Println(err.Error())
			os.Exit(1)
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		gormDB := db.WithContext(ctx)
		go func() {
			if err := user.Create(gormDB, superuser); err != nil {
				panic(err)
			}
			cancel()
		}()
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			pterm.Error.Println("create user timeout")
			os.Exit(1)
		} else {
			pterm.Success.Printf("create user %s success\n", user.Username)
			os.Exit(0)
		}
	},
}

var forceResetPasswordCmd = &cobra.Command{
	Use:   "force-reset-password",
	Short: "Force reset user's password by command line",
	Run: func(cmd *cobra.Command, args []string) {
		flags := cmd.Flags()
		timeout, err := flags.GetDuration("timeout")
		if err != nil {
			panic(fmt.Sprintf("invalid timeout: %v", err))
		}
		_, injector := cmdToolBox(cmd)
		// Force validater initialization
		provider.MustInvokeValidate(injector)
		db := provider.MustInvokeCoreDatabase(injector)
		// Force rbac initialization
		provider.MustInvokeRbac(injector)
		username, password, err := extractUsernamePassword()
		if err != nil {
			pterm.Error.Println(err.Error())
			os.Exit(1)
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		gormDB := db.WithContext(ctx)
		go func() {
			var user core.User
			if err := gormDB.Take(&user, &core.User{Username: username}).Error; err != nil {
				panic(err)
			} else if err := user.ResetPassword(gormDB, password); err != nil {
				panic(err)
			}
			cancel()
		}()
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			pterm.Error.Println("force reset password timeout")
			os.Exit(1)
		} else {
			pterm.Success.Printf("reset user %s's password success\n", username)
			os.Exit(0)
		}
	},
}

func init() {
	flags := createUserCmd.Flags()
	flags.Duration("timeout", time.Minute, "timeout for inserting data into database")
	flags.Bool("superuser", false, "create as superuser")

	flags = forceResetPasswordCmd.Flags()
	flags.Duration("timeout", time.Minute, "timeout for inserting data into database")
}
