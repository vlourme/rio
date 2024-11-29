//go:build !linux

package aio

func supportsMultipathTCP() bool {
	return false
}

func tryGetMultipathTCPProto() int {
	return 0
}

func IsUsingMultipathTCP(fd NetFd) bool {
	return false
}
