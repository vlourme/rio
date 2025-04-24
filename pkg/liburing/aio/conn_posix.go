//go:build !linux

package aio

type Conn struct{}

func OOBLen() int {
	return 0
}
