package bytebufferpool

type AreaOfBuffer interface {
	Bytes() []byte
	Finish()
}

type areaOfBuffer struct {
	p   []byte
	fin func()
}

func (area *areaOfBuffer) Bytes() []byte {
	return area.p
}

func (area *areaOfBuffer) Finish() {
	area.fin()
}
