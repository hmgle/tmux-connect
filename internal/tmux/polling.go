package tmux

import (
	"context"
	"strings"
	"time"
)

func (c *Client) startPollingSubscription(ctx context.Context, pane PaneInfo, lines int) *Subscription {
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

		var previous string
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
	prevLines := strings.Split(previous, "\n")
	currLines := strings.Split(current, "\n")
	maxOverlap := min(len(prevLines), len(currLines))
	for overlap := maxOverlap; overlap > 0; overlap-- {
		if slicesEqual(prevLines[len(prevLines)-overlap:], currLines[:overlap]) {
			return strings.Join(currLines[overlap:], "\n")
		}
	}
	return current
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

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
