//go:build linux

package aio

import "golang.org/x/sys/unix"

func KernelVersion() (major, minor int) {
	var uname unix.Utsname
	if err := unix.Uname(&uname); err != nil {
		return
	}
	var (
		values    [2]int
		value, vi int
	)
	for _, c := range uname.Release {
		if '0' <= c && c <= '9' {
			value = (value * 10) + int(c-'0')
		} else {
			values[vi] = value
			vi++
			if vi >= len(values) {
				break
			}
			value = 0
		}
	}

	return values[0], values[1]
}
