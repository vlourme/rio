//go:build !linux

package aio

func tryGetMultipathTCPProto() (int, bool) {
	return 0, false
}

func IsUsingMultipathTCP(fd NetFd) bool {
	return false
}
