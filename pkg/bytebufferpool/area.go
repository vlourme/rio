package bytebufferpool

type AreaOfBuffer interface {
	Bytes() []byte
	Finish()
	Cancel()
}

type areaOfBuffer struct {
	p      []byte
	finish func()
	cancel func()
}

func (area *areaOfBuffer) Bytes() []byte {
	return area.p
}

func (area *areaOfBuffer) Finish() {
	area.finish()
}

func (area *areaOfBuffer) Cancel() {
	area.cancel()
}
