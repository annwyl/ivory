package ivory

import (
	"encoding/json"
	"fmt"
)

type Store interface {
	Save(key string, record map[string]any) error
	Close() error
}

// optional interface, if implemented the store can be searched and exported with args
type Queryable interface {
	Query(term string, limit int) ([]map[string]any, error)
}

type StoreFactory func(config json.RawMessage) (Store, error)

var registeredStores = make(map[string]StoreFactory)

func RegisterStore(name string, factory StoreFactory) error {
	if _, ok := registeredStores[name]; ok {
		return fmt.Errorf("store already registered: %s", name)
	}
	registeredStores[name] = factory
	return nil
}

func GetRegisteredStores() map[string]StoreFactory {
	return registeredStores
}

func getStore(config Config) (Store, error) {
	factory, ok := registeredStores[config.Store]
	if !ok {
		return nil, fmt.Errorf("unknown store: %s", config.Store)
	}
	return factory(config.StoreConfig)
}
