package ivory

import (
	"context"
	"fmt"
)

type Crawler interface {
	Name() string
	Run(ctx context.Context, rt *Runtime) error
}

// optional interface for crawlers to expose their own settings
type Describable interface {
	Describe() map[string]string
}

type CrawlerFactory func() Crawler

var registeredCrawlers = make(map[string]CrawlerFactory)

func RegisterCrawler(name string, factory CrawlerFactory) error {
	if _, ok := registeredCrawlers[name]; ok {
		return fmt.Errorf("crawler already registered: %s", name)
	}
	registeredCrawlers[name] = factory
	return nil
}

func GetRegisteredCrawlers() map[string]CrawlerFactory {
	return registeredCrawlers
}

func getFactory(name string) (CrawlerFactory, error) {
	factory, ok := registeredCrawlers[name]
	if !ok {
		return nil, fmt.Errorf("unknown crawler: %s", name)
	}
	return factory, nil
}
