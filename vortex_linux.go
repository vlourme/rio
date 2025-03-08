//go:build linux

package rio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/process"
	"sync"
)

// UseProcessPriority
// 设置进程等级
func UseProcessPriority(level process.PriorityLevel) {
	_ = process.SetCurrentProcessPriority(level)
}

// PrepareIOURingSetupOptions
// 准备 IOURING 的设置选项，必须在 Pin、 Dial 、 Listen 之前。
func PrepareIOURingSetupOptions(options ...aio.Option) {
	aio.PrepareInitIOURingOptions(options...)
}

// Pin
// 钉住 IOURING 。
// 一般用于程序启动时。
// 这用手动管理 IOURING 的生命周期，一般用于只有 Dial 的使用。
// 注意：必须 Unpin 来关闭 IOURING 。
func Pin() (err error) {
	pinnedLock.Lock()
	defer pinnedLock.Unlock()
	pinned, err = aio.Acquire()
	return
}

// Unpin
// 取钉。
// 用于关闭 IOURING。
func Unpin() (err error) {
	pinnedLock.Lock()
	defer pinnedLock.Unlock()
	if pinned == nil {
		err = errors.New("unpin failed cause not pinned")
		return
	}
	err = aio.Release(pinned)
	return
}

var (
	pinned     *aio.Vortex = nil
	pinnedLock sync.Mutex
)
