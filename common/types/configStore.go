package types

type DynamicConfigBlob struct {
	SchemaVersion int64                 `json:"schemaVersion,omitempty"`
	Entries       []*DynamicConfigEntry `json:"entries,omitempty"`
}

type DynamicConfigEntry struct {
	Name   string                `json:"name,omitempty"`
	Values []*DynamicConfigValue `json:"values,omitempty"`
}

type DynamicConfigValue struct {
	Value   *DataBlob              `json:"value,omitempty"`
	Filters []*DynamicConfigFilter `json:"filters,omitempty"`
}

type DynamicConfigFilter struct {
	Name  string    `json:"name,omitempty"`
	Value *DataBlob `json:"value,omitempty"`
}

func (dcf *DynamicConfigFilter) Copy() *DynamicConfigFilter {
	if dcf == nil {
		return nil
	}
	return &DynamicConfigFilter{
		Name:  dcf.Name,
		Value: dcf.Value.DeepCopy(),
	}
}

func (dcv *DynamicConfigValue) Copy() *DynamicConfigValue {
	if dcv == nil {
		return nil
	}

	var newFilters []*DynamicConfigFilter
	if dcv.Filters != nil {
		newFilters = make([]*DynamicConfigFilter, 0, len(dcv.Filters))
		for _, filter := range dcv.Filters {
			newFilters = append(newFilters, filter.Copy())
		}
	}

	return &DynamicConfigValue{
		Value:   dcv.Value.DeepCopy(),
		Filters: newFilters,
	}
}

func (dce *DynamicConfigEntry) Copy() *DynamicConfigEntry {
	if dce == nil {
		return nil
	}

	var newValues []*DynamicConfigValue
	if dce.Values != nil {
		newValues = make([]*DynamicConfigValue, 0, len(dce.Values))
		for _, value := range dce.Values {
			newValues = append(newValues, value.Copy())
		}
	}

	return &DynamicConfigEntry{
		Name:   dce.Name,
		Values: newValues,
	}
}
