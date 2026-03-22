package tmux

import (
	"fmt"
	"strings"
)

// CutoverSubscription trims the buffered prefix already reflected in the
// initial snapshot and forwards the remaining stream through a fresh wrapper.
func CutoverSubscription(base *Subscription, initial string) (*Subscription, error) {
	if base == nil {
		return nil, fmt.Errorf("subscription is required")
	}

	pending, chunks, errs, err := drainBufferedSubscription(base)
	if err != nil {
		_ = base.Close()
		return nil, err
	}
	pending = trimSnapshotOverlap(initial, pending)

	out := &Subscription{
		chunks: make(chan OutputChunk, maxInt(cap(base.chunks), len(pending))),
		errs:   make(chan error, cap(base.errs)),
		close:  base.Close,
	}
	for _, chunk := range pending {
		out.chunks <- chunk
	}
	if chunks == nil && errs == nil {
		close(out.chunks)
		close(out.errs)
		return out, nil
	}

	go func() {
		defer close(out.chunks)
		defer close(out.errs)

		for {
			if chunks == nil && errs == nil {
				return
			}
			select {
			case err, ok := <-errs:
				if !ok {
					errs = nil
					continue
				}
				if err == nil {
					continue
				}
				select {
				case out.errs <- err:
				default:
				}
			case chunk, ok := <-chunks:
				if !ok {
					chunks = nil
					continue
				}
				out.chunks <- chunk
			}
		}
	}()

	return out, nil
}

func drainBufferedSubscription(base *Subscription) ([]OutputChunk, <-chan OutputChunk, <-chan error, error) {
	chunks := base.chunks
	errs := base.errs
	var pending []OutputChunk
	for {
		progressed := false

		select {
		case err, ok := <-errs:
			progressed = true
			if !ok {
				errs = nil
				break
			}
			if err != nil {
				return nil, nil, nil, err
			}
		default:
		}

		select {
		case chunk, ok := <-chunks:
			progressed = true
			if !ok {
				chunks = nil
				break
			}
			pending = append(pending, chunk)
		default:
		}

		if chunks == nil && errs == nil {
			return pending, nil, nil, nil
		}
		if !progressed {
			return pending, chunks, errs, nil
		}
	}
}

func trimSnapshotOverlap(initial string, pending []OutputChunk) []OutputChunk {
	if initial == "" || len(pending) == 0 {
		return pending
	}
	var buffered strings.Builder
	for _, chunk := range pending {
		buffered.WriteString(chunk.Text)
	}
	trimBytes := suffixPrefixOverlap(initial, buffered.String())
	if trimBytes == 0 {
		return pending
	}
	return trimChunkPrefix(pending, trimBytes)
}

func suffixPrefixOverlap(snapshot string, buffered string) int {
	maxOverlap := len(snapshot)
	if len(buffered) < maxOverlap {
		maxOverlap = len(buffered)
	}
	for overlap := maxOverlap; overlap > 0; overlap-- {
		if snapshot[len(snapshot)-overlap:] == buffered[:overlap] {
			return overlap
		}
	}
	return 0
}

func trimChunkPrefix(chunks []OutputChunk, trimBytes int) []OutputChunk {
	if trimBytes <= 0 {
		return chunks
	}
	trimmed := make([]OutputChunk, 0, len(chunks))
	remaining := trimBytes
	for _, chunk := range chunks {
		text := chunk.Text
		if remaining >= len(text) {
			remaining -= len(text)
			continue
		}
		if remaining > 0 {
			chunk.Text = text[remaining:]
			remaining = 0
		}
		if chunk.Text != "" {
			trimmed = append(trimmed, chunk)
		}
	}
	return trimmed
}
