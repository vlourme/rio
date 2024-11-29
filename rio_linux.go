//go:build linux

package rio

import "github.com/brickingsoft/rio/pkg/aio"

type IOURingSettings struct {
	aio.IOURingSettings
}

func WithIOURingSettings(settings IOURingSettings) StartupOption {
	return func(o *StartupOptions) error {
		o.AIOOptions.Settings = settings
		return nil
	}
}
