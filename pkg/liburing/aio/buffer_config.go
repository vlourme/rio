package aio

import "time"

type BufferAndRingConfig struct {
	Size        int
	Count       int
	IdleTimeout time.Duration
	mask        uint16
}
