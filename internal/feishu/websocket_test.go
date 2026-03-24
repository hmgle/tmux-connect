package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

func TestWSFragmentAccumulatorCombineOutOfOrder(t *testing.T) {
	t.Parallel()

	acc := newWSFragmentAccumulator()
	if payload, ok := acc.Combine("msg-1", 3, 2, []byte("c")); ok || payload != nil {
		t.Fatalf("third fragment early = (%q, %v), want (nil, false)", payload, ok)
	}
	if payload, ok := acc.Combine("msg-1", 3, 0, []byte("a")); ok || payload != nil {
		t.Fatalf("first fragment incomplete = (%q, %v), want (nil, false)", payload, ok)
	}
	payload, ok := acc.Combine("msg-1", 3, 1, []byte("b"))
	if !ok {
		t.Fatal("Combine() ok = false, want true on final fragment")
	}
	if string(payload) != "abc" {
		t.Fatalf("payload = %q, want abc", payload)
	}
}

func TestWSFragmentAccumulatorCombineRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	acc := newWSFragmentAccumulator()
	cases := []struct {
		name      string
		messageID string
		sum       int
		seq       int
	}{
		{name: "missing_message_id", sum: 2, seq: 0},
		{name: "negative_seq", messageID: "msg-1", sum: 2, seq: -1},
		{name: "seq_out_of_range", messageID: "msg-1", sum: 2, seq: 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if payload, ok := acc.Combine(tc.messageID, tc.sum, tc.seq, []byte("x")); ok || payload != nil {
				t.Fatalf("Combine() = (%q, %v), want (nil, false)", payload, ok)
			}
		})
	}
}

func TestWSRuntimeConfigShouldRetry(t *testing.T) {
	t.Parallel()

	infinite := defaultWSRuntimeConfig()
	infinite.ReconnectCount = -1
	if !infinite.shouldRetry(100) {
		t.Fatal("shouldRetry() = false, want true for infinite retries")
	}

	limited := defaultWSRuntimeConfig()
	limited.ReconnectCount = 2
	if !limited.shouldRetry(0) {
		t.Fatal("shouldRetry(0) = false, want true")
	}
	if !limited.shouldRetry(1) {
		t.Fatal("shouldRetry(1) = false, want true")
	}
	if limited.shouldRetry(2) {
		t.Fatal("shouldRetry(2) = true, want false")
	}
}

func TestWSRuntimeConfigWaitRetryHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	cfg := defaultWSRuntimeConfig()
	cfg.ReconnectInterval = time.Second
	cfg.ReconnectNonce = 0

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := cfg.waitRetry(ctx, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitRetry() error = %v, want context.Canceled", err)
	}
}

func TestWSRuntimeConfigApplyUpdatesPingInterval(t *testing.T) {
	t.Parallel()

	cfg := defaultWSRuntimeConfig()
	cfg.apply(&larkws.ClientConfig{PingInterval: 7})

	if got := cfg.pingInterval(); got != 7*time.Second {
		t.Fatalf("pingInterval = %v, want 7s", got)
	}
}

func TestWSSessionHandleControlFrameAppliesRuntimeConfig(t *testing.T) {
	t.Parallel()

	cfg := defaultWSRuntimeConfig()
	session := &wsSession{
		client: &Client{logger: newSDKLogger(nil)},
		config: cfg,
	}

	payload, err := json.Marshal(larkws.ClientConfig{
		ReconnectCount:    3,
		ReconnectInterval: 11,
		ReconnectNonce:    5,
		PingInterval:      13,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	headers := larkws.Headers{}
	headers.Add(larkws.HeaderType, string(larkws.MessageTypePong))
	session.handleControlFrame(context.Background(), larkws.Frame{
		Headers: headers,
		Payload: payload,
	})

	if !cfg.shouldRetry(2) || cfg.shouldRetry(3) {
		t.Fatalf("ReconnectCount not applied correctly")
	}
	if got := cfg.pingInterval(); got != 13*time.Second {
		t.Fatalf("pingInterval = %v, want 13s", got)
	}
}
