package daemon

import (
	"fmt"
	"io"
	"slices"
	"strings"
)

type platformAdapterFactory func(Config, io.Writer, *Store) (platformAdapter, error)

var platformAdapterFactories = map[string]platformAdapterFactory{}

var platformOrder = []string{
	"telegram",
	"feishu",
	"slack",
	"discord",
	"whatsapp",
}

func RegisterPlatformAdapter(name string, factory platformAdapterFactory) {
	name = normalizePlatformName(name)
	if name == "" {
		panic("daemon: register platform adapter with empty name")
	}
	if factory == nil {
		panic("daemon: register platform adapter with nil factory")
	}
	platformAdapterFactories[name] = factory
}

func newPlatformAdapter(cfg Config, stderr io.Writer, store *Store) (platformAdapter, error) {
	name := normalizePlatformName(cfg.Platform)
	factory, ok := platformAdapterFactories[name]
	if !ok {
		return nil, unsupportedPlatformError(name)
	}
	return factory(cfg, stderr, store)
}

func defaultPlatformName() string {
	if isPlatformAvailable("telegram") {
		return "telegram"
	}
	platforms := availablePlatformNames()
	if len(platforms) == 0 {
		return ""
	}
	return platforms[0]
}

func availablePlatformNames() []string {
	names := make([]string, 0, len(platformAdapterFactories))
	for name := range platformAdapterFactories {
		names = append(names, name)
	}
	slices.SortFunc(names, comparePlatformName)
	return names
}

func availablePlatformChoices() string {
	return strings.Join(availablePlatformNames(), "|")
}

func availablePlatformSummary() string {
	return strings.Join(availablePlatformNames(), ", ")
}

func isPlatformAvailable(name string) bool {
	_, ok := platformAdapterFactories[normalizePlatformName(name)]
	return ok
}

func unsupportedPlatformError(name string) error {
	name = normalizePlatformName(name)
	switch platforms := availablePlatformSummary(); platforms {
	case "":
		return fmt.Errorf("unsupported --platform %q: this build has no remote platforms compiled in", name)
	default:
		return fmt.Errorf("unsupported --platform %q: compiled platforms: %s", name, platforms)
	}
}

func normalizePlatformName(name string) string {
	return strings.TrimSpace(strings.ToLower(name))
}

func comparePlatformName(left string, right string) int {
	leftRank := platformRank(left)
	rightRank := platformRank(right)
	switch {
	case leftRank != rightRank:
		if leftRank < rightRank {
			return -1
		}
		return 1
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func platformRank(name string) int {
	for idx, candidate := range platformOrder {
		if candidate == name {
			return idx
		}
	}
	return len(platformOrder)
}
