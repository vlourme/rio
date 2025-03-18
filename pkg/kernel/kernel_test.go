package kernel_test

import (
	"github.com/brickingsoft/rio/pkg/kernel"
	"testing"
)

func TestGet(t *testing.T) {
	v := kernel.Get()
	if v.Invalidate() {
		t.Fatal("get value failed")
	}
	t.Log(v)
}

func TestEnable(t *testing.T) {
	v := kernel.Get()
	if v.Invalidate() {
		t.Fatal("get value failed")
	}
	t.Log(v)
	t.Log(kernel.Enable(5, 19, 0))
	t.Log(kernel.Enable(6, 6, 36))
	t.Log(kernel.Enable(6, 19, 0))
}

func TestVersion_GTE(t *testing.T) {
	v := kernel.Get()
	if v.Invalidate() {
		t.Fatal("get value failed")
	}
	t.Log(v)
	t.Log("GTE")
	t.Log(v.GTE(5, 19, 0))
	t.Log(v.GTE(6, 6, 36))
	t.Log(v.GTE(6, 19, 0))
	t.Log("LTE")
	t.Log(v.LT(5, 19, 0))
	t.Log(v.LT(6, 6, 36))
	t.Log(v.LT(6, 19, 0))
}
