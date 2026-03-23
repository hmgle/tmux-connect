package daemon

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strings"
)

type platformAdapterFactory func(Config, io.Writer, *Store) (platformAdapter, error)
type platformConfigValidator func(Config) error
type platformDoctorFunc func(io.Writer, Config) error

type platformRegistration struct {
	factory     platformAdapterFactory
	validateRun platformConfigValidator
	doctor      platformDoctorFunc
}

var platformRegistrations = map[string]platformRegistration{}

var platformOrder = []string{
	"telegram",
	"feishu",
	"slack",
	"discord",
	"whatsapp",
}

func RegisterPlatform(name string, registration platformRegistration) {
	name = normalizePlatformName(name)
	if name == "" {
		panic("daemon: register platform adapter with empty name")
	}
	if registration.factory == nil {
		panic("daemon: register platform adapter with nil factory")
	}
	platformRegistrations[name] = registration
}

func registeredPlatform(name string) (platformRegistration, bool) {
	registration, ok := platformRegistrations[normalizePlatformName(name)]
	return registration, ok
}

func newPlatformAdapter(cfg Config, stderr io.Writer, store *Store) (platformAdapter, error) {
	name := normalizePlatformName(cfg.Platform)
	registration, ok := registeredPlatform(name)
	if !ok {
		return nil, unsupportedPlatformError(name)
	}
	return registration.factory(cfg, stderr, store)
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
	names := make([]string, 0, len(platformRegistrations))
	for name := range platformRegistrations {
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
	_, ok := registeredPlatform(name)
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
	if c := cmp.Compare(platformRank(left), platformRank(right)); c != 0 {
		return c
	}
	return cmp.Compare(left, right)
}

func platformRank(name string) int {
	if idx := slices.Index(platformOrder, name); idx >= 0 {
		return idx
	}
	return len(platformOrder)
}
