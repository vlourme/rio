package kernel_test

import (
	"github.com/brickingsoft/rio/pkg/iouring"
	"testing"
)

func TestGetKernelVersion(t *testing.T) {
	v, err := iouring.GetKernelVersion()
	if err != nil {
		t.Fatal(err)
	}
	t.Log(v)
}
