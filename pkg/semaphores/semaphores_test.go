package semaphores_test

import (
	"context"
	"github.com/brickingsoft/rio/pkg/semaphores"
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	sh, _ := semaphores.New(1 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	wg := new(sync.WaitGroup)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(ctx context.Context, wg *sync.WaitGroup, sh *semaphores.Semaphores) {
			defer wg.Done()
			err := sh.Wait(ctx)
			t.Log("goroutine exit", err)
		}(ctx, wg, sh)
	}
	cancel()
	wg.Wait()
}
