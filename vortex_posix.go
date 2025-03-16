//go:build !linux

package rio

import (
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/process"
)

// UseProcessPriority
// 设置进程等级
func UseProcessPriority(level process.PriorityLevel) {
	_ = process.SetCurrentProcessPriority(level)
}

// Presets
// 准备 IOURING 的设置选项，必须在 Pin、 Dial 、 Listen 之前。
func Presets(_ ...aio.Option) {}

// Pin
// 钉住 IOURING 。
// 一般用于程序启动时。
// 这用手动管理 IOURING 的生命周期，一般用于只有 Dial 的使用。
// 注意：必须 Unpin 来关闭 IOURING 。
func Pin() error {
	return nil
}

// Unpin
// 取钉。
// 用于关闭 IOURING。
func Unpin() error {
	return nil
}
