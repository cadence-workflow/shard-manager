package testflags

import (
	"fmt"
	"os"
	"testing"
)

// Pass these testflags in as environment variables in order to run tests marked as needing an
// external dependency.
// Tests can be marked as having an external dependency by calling the Require* functions below.
// In order to start external dependencies, see documentation at https://github.com/cadence-workflow/shard-manager

var (
	cassandra = "CASSANDRA"
	mongodb   = "MONGODB"
	mysql     = "MYSQL"
	postgres  = "POSTGRES"
	etcd      = "ETCD"
)

// NOTE: We are using environment variables instead of go testflags or go build directives, because we:
// 1) Want tests to be built all the time (go build directives don't give us that)
// 2) Want tests to be marked as skipped (go build directives don't give us that)
// 3) Want to be able to run individual tests (go testflags errors if the test doesn't import
// something that defines the testflags.)

func RequireMySQL(t *testing.T) {
	require(t, mysql)
}

func RequirePostgres(t *testing.T) {
	require(t, postgres)
}

func RequireMongoDB(t *testing.T) {
	require(t, mongodb)
}

func RequireCassandra(t *testing.T) {
	require(t, cassandra)
}

func RequireEtcd(t *testing.T) {
	require(t, etcd)
}

func require(t *testing.T, name string) {
	if !checkEnv(name) {
		t.Skip(fmt.Sprintf("Skipping test that requires %s to run - start %s and set '%s=1' environment variable to run this test.",
			name, name, name))
	}
}

func checkEnv(name string) bool {
	return os.Getenv(name) != ""
}
