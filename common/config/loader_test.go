package config

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type (
	LoaderSuite struct {
		*require.Assertions
		suite.Suite
	}

	itemsConfig struct {
		Item1 string `yaml:"item1"`
		Item2 string `yaml:"item2"`
	}

	testConfig struct {
		Items itemsConfig `yaml:"items"`
	}
)

func TestLoaderSuite(t *testing.T) {
	suite.Run(t, new(LoaderSuite))
}

func (s *LoaderSuite) SetupTest() {
	s.Assertions = require.New(s.T())
}

func (s *LoaderSuite) TestBaseYaml() {

	dir := s.T().TempDir()

	s.createFile(dir, "base.yaml", `
items:
  item1: base1
  item2: base2`)

	envs := []string{"", "prod"}
	zones := []string{"", "us-east-1a"}

	for _, env := range envs {
		for _, zone := range zones {
			var cfg testConfig
			err := Load(env, dir, zone, &cfg)
			s.Nil(err)
			s.Equal("base1", cfg.Items.Item1)
			s.Equal("base2", cfg.Items.Item2)
		}
	}
}

func (s *LoaderSuite) TestHierarchy() {

	dir := s.T().TempDir()

	s.createFile(dir, "base.yaml", `
items:
  item1: base1
  item2: base2`)

	s.createFile(dir, "development.yaml", `
items:
  item1: development1`)

	s.createFile(dir, "prod.yaml", `
items:
  item1: prod1`)

	s.createFile(dir, "prod_dca.yaml", `
items:
  item1: prod_dca1`)

	testCases := []struct {
		env   string
		zone  string
		item1 string
		item2 string
	}{
		{"", "", "development1", "base2"},
		{"", "dca", "development1", "base2"},
		{"", "pdx", "development1", "base2"},
		{"development", "", "development1", "base2"},
		{"development", "dca", "development1", "base2"},
		{"development", "pdx", "development1", "base2"},
		{"prod", "", "prod1", "base2"},
		{"prod", "dca", "prod_dca1", "base2"},
		{"prod", "pdx", "prod1", "base2"},
	}

	for _, tc := range testCases {
		var cfg testConfig
		err := Load(tc.env, dir, tc.zone, &cfg)
		s.Nil(err)
		s.Equal(tc.item1, cfg.Items.Item1)
		s.Equal(tc.item2, cfg.Items.Item2)
	}
}

func (s *LoaderSuite) TestInvalidPath() {
	var cfg testConfig
	err := Load("prod", "", "", &cfg)
	s.NotNil(err)
}

func (s *LoaderSuite) createFile(dir string, file string, content string) {
	err := ioutil.WriteFile(path(dir, file), []byte(content), fileMode)
	s.Nil(err)
}
