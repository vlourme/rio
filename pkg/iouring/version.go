package iouring

const (
	LibraryVersionMajor = 2
	LibraryVersionMinor = 5
)

// liburing: io_uring_major_version - https://manpages.debian.org/unstable/liburing-dev/io_uring_major_version.3.en.html
func MajorVersion() int {
	return LibraryVersionMajor
}

// liburing: io_uring_minor_version - https://manpages.debian.org/unstable/liburing-dev/io_uring_minor_version.3.en.html
func MinorVersion() int {
	return LibraryVersionMinor
}

// liburing: io_uring_check_version - https://manpages.debian.org/unstable/liburing-dev/io_uring_check_version.3.en.html
func CheckVersion(major, minor int) bool {
	return major > MajorVersion() ||
		(major == MajorVersion() && minor >= MinorVersion())
}
