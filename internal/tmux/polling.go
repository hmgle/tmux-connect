package tmux

import (
	"context"
	"strings"
	"time"
)

func (c *Client) startPollingSubscription(ctx context.Context, pane PaneInfo, lines int) *Subscription {
	return c.startPollingSubscriptionWithBaseline(ctx, pane, lines, "")
}

func (c *Client) startPollingSubscriptionWithBaseline(ctx context.Context, pane PaneInfo, lines int, previous string) *Subscription {
	subCtx, cancel := context.WithCancel(ctx)
	sub := &Subscription{
		chunks: make(chan OutputChunk, 4),
		errs:   make(chan error, 1),
		close: func() error {
			cancel()
			return nil
		},
	}
	go func() {
		defer close(sub.chunks)
		defer close(sub.errs)
		defer cancel()

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			body, err := c.CapturePane(subCtx, pane.Target, lines)
			if err != nil {
				if subCtx.Err() == nil {
					sub.errs <- err
				}
				return
			}
			if previous == "" {
				previous = body
			} else if body != previous {
				diff := snapshotDiff(previous, body)
				if diff != "" {
					sub.chunks <- OutputChunk{
						Target:     pane.Target,
						Text:       diff,
						ReceivedAt: time.Now(),
					}
				}
				previous = body
			}

			select {
			case <-subCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return sub
}

func snapshotDiff(previous string, current string) string {
	if previous == current {
		return ""
	}

	if strings.HasPrefix(current, previous) {
		return trimLeadingSnapshotDelta(current[len(previous):])
	}

	if diff, ok := snapshotTailLineDiff(previous, current); ok {
		return diff
	}

	prevLines := strings.Split(previous, "\n")
	currLines := strings.Split(current, "\n")
	maxOverlap := min(len(prevLines), len(currLines))
	for overlap := maxOverlap; overlap > 0; overlap-- {
		if slicesEqual(prevLines[len(prevLines)-overlap:], currLines[:overlap]) {
			return trimLeadingSnapshotDelta(strings.Join(currLines[overlap:], "\n"))
		}
	}
	return current
}

func snapshotTailLineDiff(previous string, current string) (string, bool) {
	prevLines := snapshotLines(previous)
	currLines := snapshotLines(current)
	if len(prevLines) == 0 || len(currLines) == 0 {
		return "", false
	}

	shared := commonPrefixLines(prevLines, currLines)
	if shared != len(prevLines)-1 {
		return "", false
	}
	if shared >= len(currLines) {
		return "", false
	}
	if !strings.HasPrefix(currLines[shared], prevLines[shared]) {
		return "", false
	}

	first := strings.TrimPrefix(currLines[shared], prevLines[shared])
	parts := make([]string, 0, len(currLines)-shared)
	if first != "" {
		parts = append(parts, first)
	}
	parts = append(parts, currLines[shared+1:]...)

	diff := trimLeadingSnapshotDelta(strings.Join(parts, "\n"))
	if diff == "" {
		return "", false
	}
	return diff, true
}

func snapshotLines(text string) []string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func trimLeadingSnapshotDelta(text string) string {
	return strings.TrimLeft(text, "\n")
}

func slicesEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func commonPrefixLines(left []string, right []string) int {
	limit := min(len(left), len(right))
	for i := 0; i < limit; i++ {
		if left[i] != right[i] {
			return i
		}
	}
	return limit
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
