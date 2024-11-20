//go:build !windows && !unix

package process

func SetCurrentProcessPriority(level PriorityLeven) (err error) {

	return
}
