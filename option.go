package rio

type Options struct {
}

type Option func(options *Options) (err error)
