package daemon

import "testing"

func requirePlatformAvailable(t *testing.T, name string) {
	t.Helper()
	if !isPlatformAvailable(name) {
		t.Skipf("platform %q is not compiled into this test build", name)
	}
}

func expectedAvailablePlatformNames() []string {
	names := make([]string, 0, len(platformOrder))
	for _, name := range platformOrder {
		if isPlatformAvailable(name) {
			names = append(names, name)
		}
	}
	return names
}
