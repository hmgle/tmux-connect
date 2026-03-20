package daemon

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

type paneService interface {
	List(context.Context) ([]tmuxconn.PaneRecord, error)
	Attach(context.Context, string, string, string) (tmuxconn.PaneRecord, error)
	Detach(context.Context, string) error
	Inspect(context.Context, string) (tmuxconn.PaneRecord, error)
	Snapshot(context.Context, string, int) (string, error)
	SnapshotRich(context.Context, string, int) (string, error)
	Send(context.Context, string, string, bool) error
	SendManaged(context.Context, string, string, bool) error
	SendKeys(context.Context, string, ...string) error
	SendKeysManaged(context.Context, string, ...string) error
	Enter(context.Context, string) error
	EnterManaged(context.Context, string) error
	CtrlC(context.Context, string) error
	CtrlCManaged(context.Context, string) error
	OpenStream(context.Context, string, int) (tmuxconn.PaneStream, error)
}

type PaneRegistry struct {
	service paneService

	mu         sync.RWMutex
	records    map[string]tmuxconn.PaneRecord
	managed    map[string]tmuxconn.PaneRecord
	lastErr    error
	lastLoaded bool
	dirty      bool
}

func NewPaneRegistry(service paneService) *PaneRegistry {
	return &PaneRegistry{
		service: service,
		records: make(map[string]tmuxconn.PaneRecord),
		managed: make(map[string]tmuxconn.PaneRecord),
	}
}

func (r *PaneRegistry) Refresh(ctx context.Context) error {
	records, err := r.service.List(ctx)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err != nil {
		r.lastErr = err
		return err
	}
	all := make(map[string]tmuxconn.PaneRecord, len(records))
	managed := make(map[string]tmuxconn.PaneRecord)
	for _, record := range records {
		key := record.Info.Target.PaneKey()
		all[key] = record
		if record.Metadata.Managed {
			managed[key] = record
		}
	}
	r.records = all
	r.managed = managed
	r.lastErr = nil
	r.lastLoaded = true
	r.dirty = false
	return nil
}

func (r *PaneRegistry) All() []tmuxconn.PaneRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return sortedRecords(r.records)
}

func (r *PaneRegistry) Managed() []tmuxconn.PaneRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return sortedRecords(r.managed)
}

func (r *PaneRegistry) ManagedCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.managed)
}

func (r *PaneRegistry) LastErr() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastErr
}

func (r *PaneRegistry) EnsureLoaded(ctx context.Context) error {
	r.mu.RLock()
	loaded := r.lastLoaded
	dirty := r.dirty
	r.mu.RUnlock()
	if loaded && !dirty {
		return nil
	}
	return r.Refresh(ctx)
}

func (r *PaneRegistry) MarkDirty() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dirty = true
}

func sortedRecords(source map[string]tmuxconn.PaneRecord) []tmuxconn.PaneRecord {
	records := make([]tmuxconn.PaneRecord, 0, len(source))
	for _, record := range source {
		records = append(records, record)
	}
	slices.SortFunc(records, func(a, b tmuxconn.PaneRecord) int {
		if a.Info.SessionName != b.Info.SessionName {
			return strings.Compare(a.Info.SessionName, b.Info.SessionName)
		}
		if a.Info.WindowName != b.Info.WindowName {
			return strings.Compare(a.Info.WindowName, b.Info.WindowName)
		}
		return strings.Compare(a.Info.Target.PaneKey(), b.Info.Target.PaneKey())
	})
	return records
}
