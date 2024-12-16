//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

func Sendfile(fd NetFd, filepath string, cb OperationCallback) {

}
