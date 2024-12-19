//go:build netbsd

package aio

func SetFastOpen(_ NetFd, _ int) error {
	return nil
}
