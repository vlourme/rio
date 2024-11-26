//go:build !linux

package aio

func supportsMultipathTCP() bool {
	return false
}

func tryGetMultipathTCPProto() int {
	return 0
}
