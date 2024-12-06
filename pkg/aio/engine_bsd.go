//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import "sync/atomic"

func (engine *Engine) Start() {

}

func (engine *Engine) Stop() {

}

type KqueueCylinder struct {
	kqueue  Kqueue
	actives int64
}

func (cylinder *KqueueCylinder) Fd() int {
	return cylinder.kqueue.fd
}

func (cylinder *KqueueCylinder) Loop(beg func(), end func()) {
	// todo
	// 与ring类似，prepare rw 到一个无锁queue。
	// loop 里 submit queue。注意 submit 是一次性的。
	// active 也是 queue 的 ready
	//TODO implement me
	panic("implement me")
}

func (cylinder *KqueueCylinder) Stop() {
	//TODO implement me
	panic("implement me")
}

func (cylinder *KqueueCylinder) Actives() int64 {
	return atomic.LoadInt64(&cylinder.actives)
}

type Kqueue struct {
	fd int
}
