//go:build !boringcrypto

package security

func needFIPS() bool { return false }
