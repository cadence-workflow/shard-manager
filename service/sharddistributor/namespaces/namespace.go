package namespaces

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
)

// Type determines how the shards of a namespace are defined.
type Type int

const (
	// TypeFixed namespaces have a static set of ShardNum shards.
	TypeFixed Type = iota + 1
	// TypeEphemeral namespaces have shards that are created on first request.
	TypeEphemeral
)

// String returns the name of the type as used in configuration.
func (t Type) String() string {
	switch t {
	case TypeFixed:
		return config.NamespaceTypeFixed
	case TypeEphemeral:
		return config.NamespaceTypeEphemeral
	}
	return "unknown"
}

// Namespace is a namespace served by shard-manager.
type Namespace struct {
	Name string
	Type Type
	// ShardNum is the number of shards of a fixed namespace.
	ShardNum int64
}

// IsEphemeral reports whether shards of the namespace are created on first request.
func (n Namespace) IsEphemeral() bool {
	return n.Type == TypeEphemeral
}

// FixedShards returns the shard IDs of a fixed namespace, and nil for any other type.
func (n Namespace) FixedShards() []string {
	if n.Type != TypeFixed {
		return nil
	}
	shards := make([]string, n.ShardNum)
	for i := range n.ShardNum {
		shards[i] = strconv.FormatInt(i, 10)
	}
	return shards
}

func fromConfig(ns config.Namespace) (Namespace, error) {
	if ns.Name == "" {
		return Namespace{}, errors.New("name must not be empty")
	}

	var nsType Type
	switch ns.Type {
	case config.NamespaceTypeFixed:
		if ns.ShardNum <= 0 {
			return Namespace{}, fmt.Errorf("fixed namespace must have a positive shardNum, got %d", ns.ShardNum)
		}
		nsType = TypeFixed
	case config.NamespaceTypeEphemeral:
		nsType = TypeEphemeral
	default:
		return Namespace{}, fmt.Errorf("unknown type %q, must be %q or %q", ns.Type, config.NamespaceTypeFixed, config.NamespaceTypeEphemeral)
	}

	return Namespace{Name: ns.Name, Type: nsType, ShardNum: ns.ShardNum}, nil
}
