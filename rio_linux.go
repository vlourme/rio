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

// WithSampleIOURingSettings
// 设置 IOURing。
//
// entries 最大为 2^15。
//
// flags 建议选择 aio.SetupSQPoll | aio.SetupSingleIssuer | aio.SetupDeferTaskRun。
// 不过当设置了 aio.SetupSingleIssuer | aio.SetupDeferTaskRun，则必须 WithAIOEngineCylindersLockOSThread 设置绑定线程。
func WithSampleIOURingSettings(entries uint32, flags uint32) StartupOption {
	return func(o *StartupOptions) error {
		o.AIOOptions.Settings = IOURingSettings{
			aio.IOURingSettings{
				Entries: entries,
				Param: aio.IOURingSetupParam{
					Flags: flags,
				},
			},
		}
		return nil
	}
}
