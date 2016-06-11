package main

type (
	// SparkRing is a ring structure to draw a spark line in a terminal.
	SparkRing struct {
		Ring []float64
		tail int
	}
)

var (
	steps = []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
)

// NewSparkRing will instantiate a new SparkRing with a length of len.
func NewSparkRing(len int) *SparkRing {
	return &SparkRing{
		Ring: make([]float64, len),
	}
}

// Push will push a new value to the ring.
func (s *SparkRing) Push(value float64) {
	if s.tail >= len(s.Ring) {
		s.tail = 0
	}

	s.Ring[s.tail] = value

	s.tail++
}

// Max will return the maximum value stored in the ring.
func (s *SparkRing) Max() float64 {
	var max float64

	for _, value := range s.Ring {
		if value > max {
			max = value
		}
	}

	return max
}

// String will return the sparkline as a string.
func (s *SparkRing) String() string {
	ret := make([]rune, len(s.Ring))
	stepSize := s.Max() / float64(len(steps))

	for i, value := range s.Ring {
		scaled := value / stepSize
		pos := int(scaled - 0.5)

		if pos > len(steps)-1 {
			pos = len(steps) - 1
		}

		if pos < 0 {
			pos = 0
		}

		l := len(s.Ring)
		ret[(l+i-s.tail)%l] = steps[pos]
	}

	return string(ret)
}
