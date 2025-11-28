// The MIT License (MIT)

// Copyright (c) 2017-2020 Uber Technologies Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package sqlite

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cadence-workflow/shard-manager/common/config"
	sqliteplugin "github.com/cadence-workflow/shard-manager/common/persistence/sql/sqlplugin/sqlite"
	"github.com/cadence-workflow/shard-manager/schema/sqlite"
	"github.com/cadence-workflow/shard-manager/tools/common/schema"
	"github.com/cadence-workflow/shard-manager/tools/sql"
)

// Test_SetupSchema test that setup schema works for all database sqlite schemas
// in-memory sqlite database is used for testing
func Test_SetupSchema(t *testing.T) {
	for _, dbName := range listDatabaseNames(t) {
		t.Run(dbName, func(t *testing.T) {
			conn := newInMemoryDB(t)

			err := schema.SetupFromConfig(&schema.SetupConfig{
				SchemaFilePath:    fmt.Sprintf("../../../schema/sqlite/%s/schema.sql", dbName),
				InitialVersion:    "0.1",
				Overwrite:         false,
				DisableVersioning: false,
			}, conn)

			assert.NoError(t, err)
		})
	}
}

// newInMemoryDB returns a new in-memory sqlite connection
func newInMemoryDB(t *testing.T) *sql.Connection {
	t.Helper()

	conn, err := sql.NewConnection(&config.SQL{
		PluginName: sqliteplugin.PluginName,
	})
	require.NoError(t, err)
	return conn
}

// listDatabaseSchemaFilePaths returns a list of database schema file paths
func listDatabaseNames(t *testing.T) []string {
	t.Helper()

	dirs, err := sqlite.SchemaFS.ReadDir(".")
	require.NoError(t, err)

	var databaseNames = make([]string, len(dirs))
	for i, dir := range dirs {
		databaseNames[i] = dir.Name()
	}

	return databaseNames
}
