package aio

import "github.com/brickingsoft/rio/pkg/kernel"

func CheckSendZCEnable() bool {
	return kernel.Enable(6, 0, 0)
}

func CheckSendMsdZCEnable() bool {
	return kernel.Enable(6, 1, 0)
}
