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

func WithSampleIOURingSettings(entries uint32, flags uint32, batch uint32) StartupOption {
	return func(o *StartupOptions) error {
		o.AIOOptions.Settings = IOURingSettings{
			aio.IOURingSettings{
				Entries: entries,
				Param: aio.IOURingSetupParam{
					Flags: flags,
				},
				Batch: batch,
			},
		}
		return nil
	}
}
