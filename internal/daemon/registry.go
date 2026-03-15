package daemon

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/portgle/tmux-connect/internal/tagb"
)

type paneService interface {
	List(context.Context) ([]tagb.PaneRecord, error)
	Attach(context.Context, string, string, string) (tagb.PaneRecord, error)
	Detach(context.Context, string) error
	Inspect(context.Context, string) (tagb.PaneRecord, error)
	Snapshot(context.Context, string, int) (string, error)
	Send(context.Context, string, string, bool) error
	Enter(context.Context, string) error
	CtrlC(context.Context, string) error
	OpenStream(context.Context, string, int) (tagb.PaneStream, error)
}

type PaneRegistry struct {
	service paneService

	mu         sync.RWMutex
	records    map[string]tagb.PaneRecord
	managed    map[string]tagb.PaneRecord
	lastErr    error
	lastLoaded bool
}

func NewPaneRegistry(service paneService) *PaneRegistry {
	return &PaneRegistry{
		service: service,
		records: make(map[string]tagb.PaneRecord),
		managed: make(map[string]tagb.PaneRecord),
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
	all := make(map[string]tagb.PaneRecord, len(records))
	managed := make(map[string]tagb.PaneRecord)
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
	return nil
}

func (r *PaneRegistry) All() []tagb.PaneRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return sortedRecords(r.records)
}

func (r *PaneRegistry) Managed() []tagb.PaneRecord {
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
	r.mu.RUnlock()
	if loaded {
		return nil
	}
	return r.Refresh(ctx)
}

func sortedRecords(source map[string]tagb.PaneRecord) []tagb.PaneRecord {
	records := make([]tagb.PaneRecord, 0, len(source))
	for _, record := range source {
		records = append(records, record)
	}
	slices.SortFunc(records, func(a, b tagb.PaneRecord) int {
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
