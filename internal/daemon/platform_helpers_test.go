package daemon

import "testing"

var allPlatformNames = []string{
	"telegram",
	"feishu",
	"slack",
	"discord",
	"whatsapp",
}

func requirePlatformAvailable(t *testing.T, name string) {
	t.Helper()
	if !isPlatformAvailable(name) {
		t.Skipf("platform %q is not compiled into this test build", name)
	}
}

func expectedAvailablePlatformNames() []string {
	names := make([]string, 0, len(allPlatformNames))
	for _, name := range allPlatformNames {
		if isPlatformAvailable(name) {
			names = append(names, name)
		}
	}
	return names
}
