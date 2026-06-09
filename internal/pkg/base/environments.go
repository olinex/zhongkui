package base

import (
	"fmt"
	"os"
	"strings"
)

var (
	DEBUG_ENV_VAR       bool = false
	ERROR_STACK_ENV_VAR bool = false
)

func init() {
	// DEBUG 环境变量
	tmp := strings.ToLower(os.Getenv(fmt.Sprintf(`%s_DEBUG`, strings.ToUpper(GLOBAL_PREFIX))))
	switch tmp {
	case "true", "t", "1":
		DEBUG_ENV_VAR = true
	default:
		DEBUG_ENV_VAR = false
	}

	// ERROR_STACK 环境变量
	tmp = strings.ToLower(os.Getenv(fmt.Sprintf(`%s_ERROR_STACK`, strings.ToUpper(GLOBAL_PREFIX))))
	switch tmp {
	case "true", "t", "1":
		ERROR_STACK_ENV_VAR = true
	default:
		ERROR_STACK_ENV_VAR = false
	}
}
