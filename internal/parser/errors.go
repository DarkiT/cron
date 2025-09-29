package parser

import "fmt"

// 预定义的错误类型，便于用户处理特定错误情况
var (
	// 公开错误
	ErrUnsupportedSpec = fmt.Errorf("无效的cron表达式格式")

	// 内部错误
	errInvalidSpecFormat     = fmt.Errorf("无效的cron表达式格式")
	errInvalidSpecDescriptor = fmt.Errorf("解析器不接受@描述符")
)
