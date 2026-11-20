package dynamicconfig

import (
	"errors"
	"time"

	"github.com/cadence-workflow/shard-manager/common/dynamicconfig/dynamicproperties"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/types"
)

// nopClient is a dummy implements of dynamicconfig Client interface, all operations will always return default values.
type nopClient struct{}

func (mc *nopClient) GetValue(name dynamicproperties.Key) (interface{}, error) {
	return nil, NotFoundError
}

func (mc *nopClient) GetValueWithFilters(name dynamicproperties.Key, filters map[dynamicproperties.Filter]interface{}) (interface{}, error) {
	return nil, NotFoundError
}

func (mc *nopClient) GetIntValue(name dynamicproperties.IntKey, filters map[dynamicproperties.Filter]interface{}) (int, error) {
	return name.DefaultInt(), NotFoundError
}

func (mc *nopClient) GetFloatValue(name dynamicproperties.FloatKey, filters map[dynamicproperties.Filter]interface{}) (float64, error) {
	return name.DefaultFloat(), NotFoundError
}

func (mc *nopClient) GetBoolValue(name dynamicproperties.BoolKey, filters map[dynamicproperties.Filter]interface{}) (bool, error) {
	if filters[dynamicproperties.DomainName] == "TestRawHistoryDomain" {
		return true, NotFoundError
	}
	return name.DefaultBool(), NotFoundError
}

func (mc *nopClient) GetStringValue(name dynamicproperties.StringKey, filters map[dynamicproperties.Filter]interface{}) (string, error) {
	return name.DefaultString(), NotFoundError
}

func (mc *nopClient) GetMapValue(name dynamicproperties.MapKey, filters map[dynamicproperties.Filter]interface{}) (map[string]interface{}, error) {
	return name.DefaultMap(), NotFoundError
}

func (mc *nopClient) GetDurationValue(name dynamicproperties.DurationKey, filters map[dynamicproperties.Filter]interface{}) (time.Duration, error) {
	return name.DefaultDuration(), NotFoundError
}

func (mc *nopClient) GetListValue(name dynamicproperties.ListKey, filters map[dynamicproperties.Filter]interface{}) ([]interface{}, error) {
	return name.DefaultList(), NotFoundError
}

func (mc *nopClient) UpdateValue(name dynamicproperties.Key, value interface{}) error {
	return errors.New("not supported for nop client")
}

func (mc *nopClient) RestoreValue(name dynamicproperties.Key, filters map[dynamicproperties.Filter]interface{}) error {
	return errors.New("not supported for nop client")
}

func (mc *nopClient) ListValue(name dynamicproperties.Key) ([]*types.DynamicConfigEntry, error) {
	return nil, errors.New("not supported for nop client")
}

// NewNopClient creates a nop client
func NewNopClient() Client {
	return &nopClient{}
}

// NewNopCollection creates a new nop collection
func NewNopCollection() *Collection {
	return NewCollection(&nopClient{}, log.NewNoop())
}
