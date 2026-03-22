package daemon

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func (a *slackAdapter) rememberThread(channel string, threadID string) {
	channel = strings.TrimSpace(channel)
	threadID = strings.TrimSpace(threadID)
	if channel == "" || threadID == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.timeNow()
	a.pruneExpiredLocked(now)
	a.activeThreads[channel+"|"+threadID] = now
	a.evictOverflowLocked()
}

func (a *slackAdapter) isActiveThread(channel string, threadID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.timeNow()
	a.pruneExpiredLocked(now)
	_, ok := a.activeThreads[strings.TrimSpace(channel)+"|"+strings.TrimSpace(threadID)]
	return ok
}

func (a *slackAdapter) isKnownThread(ctx context.Context, channel string, threadID string) bool {
	if a.isActiveThread(channel, threadID) {
		return true
	}
	if !a.hasPersistedThread(ctx, channel, threadID) {
		return false
	}
	a.rememberThread(channel, threadID)
	return true
}

func (a *slackAdapter) hasPersistedThread(ctx context.Context, channel string, threadID string) bool {
	if a.store == nil {
		return false
	}
	ok, err := a.store.HasThread(ctx, ChatRef{
		Platform: a.Platform(),
		ChatID:   strings.TrimSpace(channel),
	}, threadID)
	if err != nil {
		if a.stderr != nil {
			fmt.Fprintf(a.stderr, "slack thread lookup error: %v\n", err)
		}
		return false
	}
	return ok
}

func (a *slackAdapter) timeNow() time.Time {
	if a.now != nil {
		return a.now()
	}
	return time.Now()
}

func (a *slackAdapter) pruneExpiredLocked(now time.Time) {
	if a.threadTTL <= 0 {
		return
	}
	cutoff := now.Add(-a.threadTTL)
	for key, lastSeen := range a.activeThreads {
		if lastSeen.Before(cutoff) {
			delete(a.activeThreads, key)
		}
	}
}

func (a *slackAdapter) evictOverflowLocked() {
	if a.maxThreads <= 0 {
		return
	}
	for len(a.activeThreads) > a.maxThreads {
		oldestKey := ""
		var oldestSeen time.Time
		for key, lastSeen := range a.activeThreads {
			if oldestKey == "" || lastSeen.Before(oldestSeen) {
				oldestKey = key
				oldestSeen = lastSeen
			}
		}
		if oldestKey == "" {
			return
		}
		delete(a.activeThreads, oldestKey)
	}
}

func isOldSlackTimestamp(ts string) bool {
	ts = strings.TrimSpace(ts)
	if ts == "" {
		return false
	}
	dot := strings.IndexByte(ts, '.')
	if dot <= 0 {
		return false
	}
	sec, err := strconv.ParseInt(ts[:dot], 10, 64)
	if err != nil {
		return false
	}
	return time.Since(time.Unix(sec, 0)) > 2*time.Minute
}
