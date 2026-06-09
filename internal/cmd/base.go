package cmd

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/pterm/pterm"
	"github.com/sergi/go-diff/diffmatchpatch"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

func getDiffError(target, input string) error {
	if target == input {
		return nil // 无差异时直接返回
	}

	mmp := diffmatchpatch.New()

	// 生成两个字符串的差异补丁（按行对比，这里将字符串视为单行处理）
	diffs := mmp.DiffMain(target, input, false)

	// 解析差异并构建彩色输出
	var targetBuilder, inputBuilder strings.Builder
	red := color.New(color.FgRed).SprintFunc()         // 错误部分用红色
	strike := color.New(color.FgHiYellow).SprintFunc() // 多余字符用删除线

	for _, d := range diffs {
		switch d.Type {
		case diffmatchpatch.DiffEqual:
			// 相同部分直接输出
			if _, err := targetBuilder.WriteString(d.Text); err != nil {
				return errors.WithStack(err)
			}
			if _, err := inputBuilder.WriteString(d.Text); err != nil {
				return errors.WithStack(err)
			}
		case diffmatchpatch.DiffDelete:
			// 目标有、输入没有的字符（目标标红）
			if _, err := targetBuilder.WriteString(red(d.Text)); err != nil {
				return errors.WithStack(err)
			}
		case diffmatchpatch.DiffInsert:
			// 输入有、目标没有的字符（输入标红+删除线）
			if _, err := inputBuilder.WriteString(red(strike(d.Text))); err != nil {
				return errors.WithStack(err)
			}
		}
	}
	// 拼接错误信息
	return errors.Newf(
		"no match:\ntarget: %s\ninputs: %s\n",
		targetBuilder.String(),
		inputBuilder.String(),
	)
}

func staticInputDoubleCheck(input string) error {
	first, err := pterm.DefaultInteractiveTextInput.Show(
		fmt.Sprintf("please enter `%s` to confirm", input),
	)
	if err != nil {
		return errors.WithStack(err)
	}
	if err = getDiffError(input, first); err != nil {
		return errors.WithStack(err)
	}
	second, err := pterm.DefaultInteractiveTextInput.Show(
		fmt.Sprintf("please enter `%s` again to confirm", input),
	)
	if err != nil {
		return errors.WithStack(err)
	}
	if err = getDiffError(input, second); err != nil {
		return errors.WithStack(err)
	}
	if second != input {
		return errors.Newf("Invalid input %s", second)
	}
	return nil
}
