package kernel

import "fmt"

type Version struct {
	Major    int
	Minor    int
	Patch    int
	Flavor   string
	validate bool
}

func (v Version) Validate() bool {
	return v.validate
}

func (v Version) Invalidate() bool {
	return !v.validate
}

func (v Version) Compare(o Version) int {
	if v.Major > o.Major {
		return 1
	} else if v.Major < o.Major {
		return -1
	}

	if v.Minor > o.Minor {
		return 1
	} else if v.Minor < o.Minor {
		return -1
	}

	if v.Patch > o.Patch {
		return 1
	} else if v.Patch < o.Patch {
		return -1
	}
	return 0
}

func (v Version) GTE(major, minor, patch int) bool {
	return v.Compare(Version{Major: major, Minor: minor, Patch: patch}) >= 0
}

func (v Version) LT(major, minor, patch int) bool {
	return v.Compare(Version{Major: major, Minor: minor, Patch: patch}) < 0
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d%s", v.Major, v.Minor, v.Patch, v.Flavor)
}

func Enable(major, minor, patch int) bool {
	v := Get()
	if v.Invalidate() {
		return false
	}
	target := Version{
		Major:    major,
		Minor:    minor,
		Patch:    patch,
		Flavor:   "",
		validate: true,
	}
	if v.Compare(target) < 0 {
		return false
	}
	return true
}
