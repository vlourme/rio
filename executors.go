package rio

import (
	"github.com/brickingsoft/rxp"
	"runtime"
	"sync"
)

var (
	executors     rxp.Executors = nil
	executorsOnce sync.Once
)

// getExecutors
// 获取执行器
func getExecutors() rxp.Executors {
	executorsOnce.Do(func() {
		if executors == nil {
			executors = rxp.New()
			runtime.SetFinalizer(executors, rxp.Executors.CloseGracefully)
		}
	})
	return executors
}
