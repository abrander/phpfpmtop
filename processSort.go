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
		if a.State == Running {
			return true
		}
		return false
	}

	// Put the slowest requests on top.
	aReq, _ := a.RequestDuration.Float64()
	bReq, _ := b.RequestDuration.Float64()

	if aReq > bReq {
		return true
	}

	return false
}
