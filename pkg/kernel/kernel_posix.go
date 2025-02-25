//go:build !linux

package kernel

import "syscall"

func Get() (*Version, error) {
	return nil, syscall.EINVAL
}
