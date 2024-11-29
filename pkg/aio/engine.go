package aio

import (
	"fmt"
	"reflect"
	"runtime"
	"sync"
)

type Settings struct {
	QueueSize  uint32
	Flags      uint32
	Features   uint32
	Threads    uint32
	ThreadIdle uint32
}

type Options struct {
	// EngineCylinders
	// 引擎规模
	EngineCylinders int
	// Settings
	// AIO配置设置
	Settings any
}

func ResolveSettings[T any](settings any) T {
	if settings == nil {
		return *new(T)
	}
	s, ok := settings.(T)
	if !ok {
		panic(fmt.Errorf("aio: cannot resolve %s settings from %s", reflect.TypeOf(*new(T)), reflect.TypeOf(settings)))
	}
	return s
}

type Engine struct {
	fd        int
	cylinders int
	settings  any
}

var (
	_createEngineOnce         = sync.Once{}
	_engine           *Engine = nil
	_defaultOptions           = Options{
		EngineCylinders: runtime.NumCPU() * 2,
		Settings:        nil,
	}
)

func Startup(options Options) {
	cylinders := options.EngineCylinders
	if cylinders < 1 {
		cylinders = runtime.NumCPU() * 2
	}
	_engine = &Engine{
		fd:        0,
		cylinders: cylinders,
		settings:  options.Settings,
	}
	_engine.Start()
}

func Shutdown() {
	engine().Stop()
}

func engine() *Engine {
	_createEngineOnce.Do(func() {
		if _engine == nil {
			_engine = &Engine{
				fd:        0,
				cylinders: _defaultOptions.EngineCylinders,
				settings:  _defaultOptions.Settings,
			}
			_engine.Start()
			runtime.SetFinalizer(_engine, (*Engine).Stop)
		}
	})
	return _engine
}
