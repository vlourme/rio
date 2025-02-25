package kernel_test

import (
	"github.com/brickingsoft/rio/pkg/kernel"
	"testing"
)

func TestGet(t *testing.T) {
	v, err := kernel.Get()
	if err != nil {
		t.Fatal(err)
	}
	t.Log(v)
}
