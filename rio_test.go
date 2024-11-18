package rio_test

import (
	"github.com/brickingsoft/rio"
	"testing"
)

func TestStartup(t *testing.T) {
	err := rio.Startup()
	if err != nil {
		t.Fatal(err)
	}
	err = rio.Shutdown()
	if err != nil {
		t.Error(err)
	}
}
