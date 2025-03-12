package aio_test

import (
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"testing"
	"time"
)

func TestCurveTransmission_Up(t *testing.T) {
	tr := aio.NewCurveTransmission(nil)

	for i := 0; i < 10; i++ {
		n, ts := tr.Up()
		t.Log(n, time.Duration(ts.Nano()))
	}

}
