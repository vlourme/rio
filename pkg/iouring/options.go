package iouring

import "errors"

type Options struct {
	Entries      uint32
	Flags        uint32
	SQThreadCPU  uint32
	SQThreadIdle uint32
	MemoryBuffer []byte
}

type Option func(*Options) error

func WithEntries(entries uint32) Option {
	return func(o *Options) error {
		if entries > MaxEntries {
			return errors.New("entries too big")
		}
		if entries < 1 {
			entries = DefaultEntries
		}
		o.Entries = entries
		return nil
	}
}

func WithFlags(flags uint32) Option {
	return func(o *Options) error {
		o.Flags = flags
		return nil
	}
}

func WithSQThreadIdle(n uint32) Option {
	return func(o *Options) error {
		o.SQThreadIdle = n
		return nil
	}
}

func WithSQThreadCPU(cpuId uint32) Option {
	return func(o *Options) error {
		o.SQThreadCPU = cpuId
		return nil
	}
}

func WithMemoryBuffer(memoryBuffer []byte) Option {
	return func(o *Options) error {
		o.MemoryBuffer = memoryBuffer
		return nil
	}
}
