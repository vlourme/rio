//go:build windows

package rio

import "github.com/brickingsoft/rio/pkg/aio"

type IOCPSettings struct {
	aio.IOCPSettings
}

func WithIOCPSettings(settings IOCPSettings) StartupOption {
	return func(o *StartupOptions) error {
		o.AIOOptions.Settings = settings
		return nil
	}
}
