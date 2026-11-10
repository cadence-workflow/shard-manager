//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination clientInterface_mock.go -self_package github.com/cadence-workflow/shard-manager/common/dynamicconfig

package dynamicconfig

import (
	"time"

	"github.com/cadence-workflow/shard-manager/common/dynamicconfig/dynamicproperties"
	"github.com/cadence-workflow/shard-manager/common/types"
)

const (
	ConfigStoreClient = "configstore"
	FileBasedClient   = "filebased"
	InMemoryClient    = "memory"
	NopClient         = "nop"
)

// Client allows fetching values from a dynamic configuration system NOTE: This does not have async
// options right now. In the interest of keeping it minimal, we can add when requirement arises.
type Client interface {
	GetValue(name dynamicproperties.Key) (interface{}, error)
	GetValueWithFilters(name dynamicproperties.Key, filters map[dynamicproperties.Filter]interface{}) (interface{}, error)

	GetIntValue(name dynamicproperties.IntKey, filters map[dynamicproperties.Filter]interface{}) (int, error)
	GetFloatValue(name dynamicproperties.FloatKey, filters map[dynamicproperties.Filter]interface{}) (float64, error)
	GetBoolValue(name dynamicproperties.BoolKey, filters map[dynamicproperties.Filter]interface{}) (bool, error)
	GetStringValue(name dynamicproperties.StringKey, filters map[dynamicproperties.Filter]interface{}) (string, error)
	GetMapValue(name dynamicproperties.MapKey, filters map[dynamicproperties.Filter]interface{}) (map[string]interface{}, error)
	GetDurationValue(name dynamicproperties.DurationKey, filters map[dynamicproperties.Filter]interface{}) (time.Duration, error)
	GetListValue(name dynamicproperties.ListKey, filters map[dynamicproperties.Filter]interface{}) ([]interface{}, error)
	// UpdateValue takes value as map and updates by overriding. It doesn't support update with filters.
	UpdateValue(name dynamicproperties.Key, value interface{}) error
	RestoreValue(name dynamicproperties.Key, filters map[dynamicproperties.Filter]interface{}) error
	ListValue(name dynamicproperties.Key) ([]*types.DynamicConfigEntry, error)
}

var NotFoundError = &types.EntityNotExistsError{
	Message: "unable to find key",
}
