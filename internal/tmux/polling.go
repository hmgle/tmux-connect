package tmux

import (
	"context"
	"strings"
	"time"
)

func (c *Client) startPollingSubscription(ctx context.Context, pane PaneInfo) *Subscription {
	sub := &Subscription{
		chunks: make(chan OutputChunk, 4),
		errs:   make(chan error, 1),
		close:  func() error { return nil },
	}
	go func() {
		defer close(sub.chunks)
		defer close(sub.errs)

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		var previous string
		for {
			body, err := c.CapturePane(ctx, pane.Target, 120)
			if err != nil {
				if ctx.Err() == nil {
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
			case <-ctx.Done():
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
	i := 0
	for i < len(prevLines) && i < len(currLines) && prevLines[i] == currLines[i] {
		i++
	}
	return strings.Join(currLines[i:], "\n")
}
