package aio

import (
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
)

type Options struct {
	// EngineCylinders
	// 引擎规模
	EngineCylinders int
	// LoadBalance
	// 负载均衡器。 RoundRobin 和 Least。注意：windows 不支持。
	LoadBalance LoadBalanceKind
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

type Cylinder interface {
	Fd() int
	Loop(beg func(), end func())
	Stop()
	Up()
	Down()
	Actives() int64
}

type LoadBalanceKind int

const (
	RoundRobin LoadBalanceKind = iota
	Least
)

func newEngine(options Options) *Engine {
	cylinders := options.EngineCylinders
	if cylinders < 1 {
		cylinders = runtime.NumCPU() * 2
	}
	_engine = &Engine{
		fd:           0,
		settings:     options.Settings,
		loadBalancer: options.LoadBalance,
		cylindersIdx: -1,
		cylindersNum: int64(cylinders),
		cylinders:    make([]Cylinder, cylinders),
		wg:           new(sync.WaitGroup),
	}
	return _engine
}

type Engine struct {
	fd           int
	settings     any
	loadBalancer LoadBalanceKind
	cylindersIdx int64
	cylindersNum int64
	cylinders    []Cylinder
	wg           *sync.WaitGroup
}

func (engine *Engine) next() Cylinder {
	switch engine.loadBalancer {
	case RoundRobin:
		idx := atomic.AddInt64(&engine.cylindersIdx, 1) % engine.cylindersNum
		return engine.cylinders[idx]
	case Least:
		idx := 0
		actives := int64(0)
		for i, c := range engine.cylinders {
			cActives := c.Actives()
			if cActives < 1 {
				idx = i
				break
			}
			if actives < cActives {
				idx = i
			}
		}
		return engine.cylinders[idx]
	default:
		return nil
	}
}

func (engine *Engine) markCylinderLoop() {
	engine.wg.Add(1)
}

func (engine *Engine) markCylinderStop() {
	engine.wg.Done()
}

var (
	_createEngineOnce         = sync.Once{}
	_engine           *Engine = nil
	_defaultOptions           = Options{
		EngineCylinders: 0,
		LoadBalance:     RoundRobin,
		Settings:        nil,
	}
)

func Startup(options Options) {
	_engine = newEngine(options)
	_engine.Start()
}

func Shutdown() {
	engine().Stop()
}

func engine() *Engine {
	_createEngineOnce.Do(func() {
		if _engine == nil {
			_engine = newEngine(_defaultOptions)
			_engine.Start()
			runtime.SetFinalizer(_engine, (*Engine).Stop)
		}
	})
	return _engine
}

func nextCylinder() Cylinder {
	return engine().next()
}
