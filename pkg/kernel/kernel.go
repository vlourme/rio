package kernel

type Version struct {
	Kernel int
	Major  int
	Minor  int
	Flavor string
}

func Compare(a, b Version) int {
	if a.Kernel > b.Kernel {
		return 1
	} else if a.Kernel < b.Kernel {
		return -1
	}

	if a.Major > b.Major {
		return 1
	} else if a.Major < b.Major {
		return -1
	}

	if a.Minor > b.Minor {
		return 1
	} else if a.Minor < b.Minor {
		return -1
	}

	return 0
}

func Check(k, major, minor int) (bool, error) {
	var (
		v   *Version
		err error
	)
	if v, err = Get(); err != nil {
		return false, err
	}
	if Compare(*v, Version{Kernel: k, Major: major, Minor: minor}) < 0 {
		return false, nil
	}

	return true, nil
}
