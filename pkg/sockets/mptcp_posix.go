//go:build !linux

package sockets

func supportsMultipathTCP() bool {
	return false
}

func tryGetMultipathTCPProto() int {
	return 0
}
