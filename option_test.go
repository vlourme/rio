package rio_test

import (
	"fmt"
	"github.com/brickingsoft/rio"
	"github.com/brickingsoft/rxp"
	"testing"
	"time"
)

func TestOptions_AsRxpOptions(t *testing.T) {
	opts := make([]rio.Option, 0, 1)
	opts = append(opts, rio.WithCloseTimeout(1*time.Second))
	opts = append(opts, rio.WithMaxGoroutines(10))
	opts = append(opts, rio.WithMaxReadyGoroutinesIdleDuration(2*time.Second))
	opts = append(opts, rio.WithMinGOMAXPROCS(3))

	options := rio.Options{}
	for _, opt := range opts {
		err := opt(&options)
		if err != nil {
			t.Fatal(err)
		}
	}
	rops := options.AsRxpOptions()
	rps := rxp.Options{}
	for _, rop := range rops {
		err := rop(&rps)
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Log(fmt.Sprintf("%+v", rps))
}
