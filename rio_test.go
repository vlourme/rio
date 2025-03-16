package rio_test

import (
	"github.com/brickingsoft/rio"
	"testing"
)

func TestPin(t *testing.T) {
	var (
		err error
	)
	if err = rio.Pin(); err != nil {
		t.Error(err)
	}
	if err = rio.Unpin(); err != nil {
		t.Error(err)
	}
	if err = rio.Pin(); err != nil {
		t.Error(err)
	}
	if err = rio.Unpin(); err != nil {
		t.Error(err)
	}
}
