//go:build linux

package rio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/process"
	"sync"
)

func UseProcessPriority(level process.PriorityLevel) {
	_ = process.SetCurrentProcessPriority(level)
}

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
