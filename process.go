package main

import "time"

type (
	process struct {
		Pid               int       `json:"pid"`
		LastRequestCPU    float64   `json:"last request cpu"`
		LastRequestMemory int       `json:"last request memory"`
		State             string    `json:"state"`
		User              string    `json:"user"`
		ContentLength     int       `json:"content length"`
		RequestURI        string    `json:"request uri"`
		RequestDuration   int       `json:"request duration"`
		Requests          int       `json:"requests"`
		StartSince        int       `json:"start since"`
		StartTime         time.Time `json:"start time_FIXME"`
		Script            string    `json:"script"`
		RequestMethod     string    `json:"request method"`
	}
)

// Process states.
const (
	Running = "Running"
	Idle    = "Idle"
)
