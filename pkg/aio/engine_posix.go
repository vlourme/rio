//go:build !linux && !windows && !darwin && !dragonfly && !freebsd && !netbsd && !openbsd

package aio

import (
	"fmt"
	"runtime"
)

func (engine *Engine) Start() {
	panic(fmt.Errorf("aio: %s is not supported", runtime.GOOS))
}

func (engine *Engine) Stop() {
	panic(fmt.Errorf("aio: %s is not supported", runtime.GOOS))
}
