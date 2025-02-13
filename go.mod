module github.com/brickingsoft/rio

go 1.23.0

require (
	github.com/brickingsoft/rxp v1.5.14
	golang.org/x/crypto v0.33.0
	golang.org/x/sys v0.30.0
)

require github.com/brickingsoft/errors v0.5.0

require github.com/pawelgaczynski/giouring v0.0.0-20230826085535-69588b89acb9

replace github.com/pawelgaczynski/giouring v0.0.0-20230826085535-69588b89acb9 => ../../pawelgaczynski/giouring
