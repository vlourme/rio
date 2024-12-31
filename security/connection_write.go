package security

import "github.com/brickingsoft/rxp/async"

func (conn *connection) Write(b []byte) (future async.Future[int]) {
	//TODO implement me
	panic("implement me")
}

func (conn *connection) Sendfile(file string) (future async.Future[int]) {
	//TODO implement me
	panic("implement me")
}
