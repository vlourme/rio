package rio_test

import (
	"github.com/brickingsoft/rio"
	"testing"
)

func TestStartup(t *testing.T) {
	defer func() {
		if err := recover(); err != nil {
			t.Fatal(err)
		}
	}()
	rio.Startup()
	rio.Shutdown()
}
