//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing"
)

func CheckSendZCEnable() bool {
	return liburing.VersionEnable(6, 0, 0)
}

func CheckSendMsdZCEnable() bool {
	return liburing.VersionEnable(6, 1, 0)
}

func CheckMultishotAcceptEnable() bool {
	return liburing.VersionEnable(5, 19, 0)
}
