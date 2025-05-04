package bytex

import (
	"errors"
	"strconv"
	"strings"
	"unicode"
)

const (
	BYTE = 1 << (10 * iota)
	KILOBYTE
	MEGABYTE
	GIGABYTE
	TERABYTE
	PETABYTE
	EXABYTE
)

var invalidByteQuantityError = errors.New("byte quantity must be a positive integer with a unit of measurement like M, MB, MiB, G, GiB, or GB")

// FormatBytes returns a human-readable byte string of the form 10M, 12.5K, and so forth.  The following units are available:
//
//	E: Exabyte
//	P: Petabyte
//	T: Terabyte
//	G: Gigabyte
//	M: Megabyte
//	K: Kilobyte
//	B: Byte
//
// The unit that results in the smallest number greater than or equal to 1 is always chosen.
func FormatBytes(n uint64) string {
	unit := ""
	value := float64(n)

	switch {
	case n >= EXABYTE:
		unit = "E"
		value = value / EXABYTE
	case n >= PETABYTE:
		unit = "P"
		value = value / PETABYTE
	case n >= TERABYTE:
		unit = "T"
		value = value / TERABYTE
	case n >= GIGABYTE:
		unit = "G"
		value = value / GIGABYTE
	case n >= MEGABYTE:
		unit = "M"
		value = value / MEGABYTE
	case n >= KILOBYTE:
		unit = "K"
		value = value / KILOBYTE
	case n >= BYTE:
		unit = "B"
	case n == 0:
		return "0B"
	}

	result := strconv.FormatFloat(value, 'f', 1, 64)
	result = strings.TrimSuffix(result, ".0")
	return result + unit
}

// ParseBytes parses a string formatted by FormatBytes as bytes. Note binary-prefixed and SI prefixed units both mean a base-2 units
// KB = K = KiB	= 1024
// MB = M = MiB = 1024 * K
// GB = G = GiB = 1024 * M
// TB = T = TiB = 1024 * G
// PB = P = PiB = 1024 * T
// EB = E = EiB = 1024 * P
func ParseBytes(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	s = strings.ToUpper(s)

	i := strings.IndexFunc(s, unicode.IsLetter)

	if i == -1 {
		return 0, invalidByteQuantityError
	}

	bs, multiple := s[:i], s[i:]
	bs = strings.TrimSpace(bs)
	multiple = strings.TrimSpace(multiple)
	n, err := strconv.ParseFloat(bs, 64)
	if err != nil || n < 0 {
		return 0, invalidByteQuantityError
	}

	switch multiple {
	case "E", "EB", "EIB":
		return uint64(n * EXABYTE), nil
	case "P", "PB", "PIB":
		return uint64(n * PETABYTE), nil
	case "T", "TB", "TIB":
		return uint64(n * TERABYTE), nil
	case "G", "GB", "GIB":
		return uint64(n * GIGABYTE), nil
	case "M", "MB", "MIB":
		return uint64(n * MEGABYTE), nil
	case "K", "KB", "KIB":
		return uint64(n * KILOBYTE), nil
	case "B":
		return uint64(n), nil
	default:
		return 0, invalidByteQuantityError
	}
}
