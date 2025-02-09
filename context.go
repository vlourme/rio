package rio

import "context"

func Background() context.Context {
	return getExecutors().Context()
}
