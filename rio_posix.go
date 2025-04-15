//go:build !linux

package rio

// Pin aio engine, use for dial only or multi listen.
func Pin() {}

// Unpin aio engine
func Unpin() {}
