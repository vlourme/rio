package aio

import "github.com/brickingsoft/rio/pkg/kernel"

func CheckSendZCEnable() bool {
	return kernel.Enable(6, 0, 0)
}

func CheckSendMsdZCEnable() bool {
	return kernel.Enable(6, 1, 0)
}

func CheckMultishotAcceptEnable() bool {
	return kernel.Enable(5, 19, 0)
}
