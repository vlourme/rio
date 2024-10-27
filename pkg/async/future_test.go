package async_test

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/async"
	"net"
	"sync"
	"testing"
)

func TestNew(t *testing.T) {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	p := async.New[int]()
	p.Future().OnComplete(func(result int, err error) {
		t.Log("result:", result, "err:", err)
		wg.Done()
	})
	p.Succeed(1)
	wg.Wait()
}

func TestFailedFuture(t *testing.T) {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	p := async.New[net.Listener]()
	p.Future().OnComplete(func(result net.Listener, err error) {
		t.Log("result:", result, "err:", err, result == nil)
		wg.Done()
	})
	p.Fail(errors.New("fail"))
	wg.Wait()
}
