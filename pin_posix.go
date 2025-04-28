//go:build !linux

package rio

// Pin iouring ctx, use for dial only or multi listen.
func Pin() {}

// Unpin iouring ctx
func Unpin() {}
