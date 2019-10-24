package main

type (
	processSort []process
)

func (s processSort) Len() int {
	return len(s)
}

func (s processSort) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s processSort) Less(i, j int) bool {
	a := s[i]
	b := s[j]

	// If the state are different, we sort by state.
	if a.State != b.State {
		// Show running processes first.
		return a.State == Running
	}

	// Put the slowest requests on top.
	aReq, _ := a.RequestDuration.Int64()
	bReq, _ := b.RequestDuration.Int64()

	// PHP sometimes reports a duration that mostly looks like a int32
	// wraparound. We sort those last.
	if aReq > 2000000000 {
		return false
	}

	if bReq > 2000000000 {
		return true
	}

	return aReq > bReq
}
