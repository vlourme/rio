package maxprocs_test

import (
	"github.com/brickingsoft/rio/pkg/maxprocs"
	"testing"
)

func TestEnable(t *testing.T) {
	undo, err := maxprocs.Enable()
	if err != nil {
		t.Fatal(err)
		return
	}
	defer undo()
}
