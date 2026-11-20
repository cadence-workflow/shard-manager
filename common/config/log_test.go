package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
)

type LogSuite struct {
	*require.Assertions
	suite.Suite
}

func TestLogSuite(t *testing.T) {
	suite.Run(t, new(LogSuite))
}

func (s *LogSuite) SetupTest() {
	s.Assertions = require.New(s.T())
}

func (s *LogSuite) TestParseLogLevel() {
	s.Equal(zap.DebugLevel, parseZapLevel("debug"))
	s.Equal(zap.InfoLevel, parseZapLevel("info"))
	s.Equal(zap.WarnLevel, parseZapLevel("warn"))
	s.Equal(zap.ErrorLevel, parseZapLevel("error"))
	s.Equal(zap.FatalLevel, parseZapLevel("fatal"))
	s.Equal(zap.InfoLevel, parseZapLevel("unknown"))
}

func (s *LogSuite) TestNewLogger() {

	dir := s.T().TempDir()

	config := &Logger{
		Level:      "info",
		OutputFile: dir + "/test.log",
	}

	log, _ := config.NewZapLogger()
	s.NotNil(log)
	_, err := os.Stat(dir + "/test.log")
	s.Nil(err)
}
