package rio_test

import (
	"context"
	"github.com/brickingsoft/rio"
	"testing"
)

func TestStartup(t *testing.T) {
	ctx := context.Background()
	err := rio.Startup()
	if err != nil {
		t.Fatal(err)
	}
	err = rio.Executors().Execute(ctx, func() {
		t.Log("do...")
	})
	if err != nil {
		t.Error(err)
	}
	err = rio.Shutdown()
	if err != nil {
		t.Error(err)
	}
}
