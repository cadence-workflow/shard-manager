package backoff

import (
	"log"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type (
	jitterSuite struct {
		suite.Suite
	}
)

func TestJitterSuite(t *testing.T) {
	s := new(jitterSuite)
	suite.Run(t, s)
}

func (s *jitterSuite) SetupSuite() {
	if testing.Verbose() {
		log.SetOutput(os.Stdout)
	}
}

func (s *jitterSuite) TestJitInt64() {
	input := int64(1048576)
	coefficient := float64(0.25)
	lowerBound := int64(float64(input) * (1 - coefficient))
	upperBound := int64(float64(input) * (1 + coefficient))

	for i := 0; i < 1048576; i++ {
		result := JitInt64(input, coefficient)
		s.True(result >= lowerBound)
		s.True(result < upperBound)
	}
}

func (s *jitterSuite) TestJitInt64WithZeroCoefficient() {
	for i := 0; i < 1048576; i++ {
		input := rand.Int63()
		s.Equal(input, JitInt64(input, 0))
	}
}

func (s *jitterSuite) TestJitInt64WithZeroInput() {
	s.Equal(int64(0), JitInt64(0, 0.5))
}

func (s *jitterSuite) TestJitFloat64() {
	input := float64(1048576.1048576)
	coefficient := float64(0.16)
	lowerBound := float64(input) * (1 - coefficient)
	upperBound := float64(input) * (1 + coefficient)

	for i := 0; i < 1048576; i++ {
		result := JitFloat64(input, coefficient)
		s.True(result >= lowerBound)
		s.True(result < upperBound)
	}
}

func (s *jitterSuite) TestJitFloat64WithZeroCoefficient() {
	for i := 0; i < 1048576; i++ {
		input := rand.Float64()
		s.Equal(input, JitFloat64(input, 0))
	}
}

func (s *jitterSuite) TestJitFloat64WithZeroInput() {
	s.Equal(float64(0), JitFloat64(0, 0.5))
}

func (s *jitterSuite) TestJitDuration() {
	input := time.Duration(1099511627776)
	coefficient := float64(0.1)
	lowerBound := time.Duration(int64(float64(input.Nanoseconds()) * (1 - coefficient)))
	upperBound := time.Duration(int64(float64(input.Nanoseconds()) * (1 + coefficient)))

	for i := 0; i < 1048576; i++ {
		result := JitDuration(input, coefficient)
		s.True(result >= lowerBound)
		s.True(result < upperBound)
	}
}

func (s *jitterSuite) TestJitDurationWithZeroCoefficient() {
	for i := 0; i < 1048576; i++ {
		input := time.Duration(rand.Int63())
		s.Equal(input, JitDuration(input, 0))
	}
}
