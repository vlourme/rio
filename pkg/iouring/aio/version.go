//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/iouring"
)

func CheckSendZCEnable() bool {
	return iouring.VersionEnable(6, 0, 0)
}

func CheckSendMsdZCEnable() bool {
	return iouring.VersionEnable(6, 1, 0)
}

func CheckMultishotAcceptEnable() bool {
	return iouring.VersionEnable(5, 19, 0)
}
