package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	ws "github.com/gorilla/websocket"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkdispatcher "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

const (
	defaultWSReconnectCount    = -1
	defaultWSReconnectInterval = 2 * time.Minute
	defaultWSPingInterval      = 2 * time.Minute
	defaultWSReconnectNonce    = 30 * time.Second
	wsFragmentTTL              = 5 * time.Second
)

type wsRuntimeConfig struct {
	mu                sync.RWMutex
	ReconnectCount    int
	ReconnectInterval time.Duration
	ReconnectNonce    time.Duration
	PingInterval      time.Duration
}

type wsFragmentAccumulator struct {
	mu     sync.Mutex
	parts  map[string][][]byte
	expiry map[string]time.Time
}

type wsSession struct {
	client     *Client
	conn       *ws.Conn
	serviceID  int32
	config     *wsRuntimeConfig
	dispatcher *larkdispatcher.EventDispatcher
	writeMu    sync.Mutex
}

func defaultWSRuntimeConfig() *wsRuntimeConfig {
	return &wsRuntimeConfig{
		ReconnectCount:    defaultWSReconnectCount,
		ReconnectInterval: defaultWSReconnectInterval,
		ReconnectNonce:    defaultWSReconnectNonce,
		PingInterval:      defaultWSPingInterval,
	}
}

func (cfg *wsRuntimeConfig) apply(remote *larkws.ClientConfig) {
	if remote == nil {
		return
	}
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	cfg.ReconnectCount = remote.ReconnectCount
	cfg.ReconnectNonce = time.Duration(remote.ReconnectNonce) * time.Second
	if remote.ReconnectInterval > 0 {
		cfg.ReconnectInterval = time.Duration(remote.ReconnectInterval) * time.Second
	}
	if remote.PingInterval > 0 {
		cfg.PingInterval = time.Duration(remote.PingInterval) * time.Second
	}
}

func (cfg *wsRuntimeConfig) shouldRetry(attempt int) bool {
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()
	return cfg.ReconnectCount < 0 || attempt < cfg.ReconnectCount
}

func (cfg *wsRuntimeConfig) waitRetry(ctx context.Context, attempt int) error {
	cfg.mu.RLock()
	wait := cfg.ReconnectInterval
	nonce := cfg.ReconnectNonce
	cfg.mu.RUnlock()

	if attempt == 0 && nonce > 0 {
		wait = time.Duration(rand.Int63n(nonce.Nanoseconds()))
	}
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (cfg *wsRuntimeConfig) pingInterval() time.Duration {
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()
	if cfg.PingInterval <= 0 {
		return defaultWSPingInterval
	}
	return cfg.PingInterval
}

func newWSFragmentAccumulator() *wsFragmentAccumulator {
	return &wsFragmentAccumulator{
		parts:  make(map[string][][]byte),
		expiry: make(map[string]time.Time),
	}
}

func (a *wsFragmentAccumulator) Combine(messageID string, sum int, seq int, payload []byte) ([]byte, bool) {
	if sum <= 1 {
		return payload, true
	}
	if messageID == "" || seq < 0 || seq >= sum {
		return nil, false
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	for key, deadline := range a.expiry {
		if now.After(deadline) {
			delete(a.parts, key)
			delete(a.expiry, key)
		}
	}

	buf := a.parts[messageID]
	if len(buf) == 0 {
		buf = make([][]byte, sum)
	}
	buf[seq] = append([]byte(nil), payload...)
	a.parts[messageID] = buf
	a.expiry[messageID] = now.Add(wsFragmentTTL)

	total := 0
	for _, part := range buf {
		if len(part) == 0 {
			return nil, false
		}
		total += len(part)
	}

	combined := make([]byte, 0, total)
	for _, part := range buf {
		combined = append(combined, part...)
	}
	delete(a.parts, messageID)
	delete(a.expiry, messageID)
	return combined, true
}

func (c *Client) runWebsocket(ctx context.Context, handler func(context.Context, MessageEvent) error) error {
	dispatcher := larkdispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(func(runCtx context.Context, event *larkim.P2MessageReceiveV1) error {
			message, ok, err := parseMessageEvent(event, c.botMentionIDs)
			if err != nil {
				c.logger.Warn(runCtx, fmt.Sprintf("feishu decode inbound message failed: %v", err))
				return err
			}
			if !ok {
				if event != nil && event.Event != nil && event.Event.Message != nil {
					msg := event.Event.Message
					msgType := strings.TrimSpace(larkcore.StringValue(msg.MessageType))
					if msgType != "" && msgType != messageTypeText {
						c.logger.Debug(runCtx, fmt.Sprintf("feishu ignore unsupported inbound message type=%s chat_id=%s message_id=%s", msgType, strings.TrimSpace(larkcore.StringValue(msg.ChatId)), strings.TrimSpace(larkcore.StringValue(msg.MessageId))))
					}
				}
				return nil
			}
			return handler(runCtx, message)
		})

	config := defaultWSRuntimeConfig()
	endpointAttempt := 0
	sessionAttempt := 0
	for {
		endpointURL, remoteConfig, err := c.fetchWebsocketEndpoint(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if !config.shouldRetry(endpointAttempt) || isFeishuClientError(err) {
				return wrapFeishuError("start websocket client", err)
			}
			c.logger.Warn(ctx, fmt.Sprintf("feishu websocket endpoint fetch failed, retrying: %v", err))
			if waitErr := config.waitRetry(ctx, endpointAttempt); waitErr != nil {
				return nil
			}
			endpointAttempt++
			continue
		}

		config.apply(remoteConfig)
		endpointAttempt = 0

		err = c.runWebsocketSession(ctx, endpointURL, config, dispatcher)
		if err == nil || ctx.Err() != nil {
			return nil
		}
		if !config.shouldRetry(sessionAttempt) || isFeishuClientError(err) {
			return wrapFeishuError("start websocket client", err)
		}
		c.logger.Warn(ctx, fmt.Sprintf("feishu websocket session ended, reconnecting: %v", err))
		if waitErr := config.waitRetry(ctx, sessionAttempt); waitErr != nil {
			return nil
		}
		sessionAttempt++
	}
}

func (c *Client) runWebsocketSession(ctx context.Context, endpointURL string, config *wsRuntimeConfig, dispatcher *larkdispatcher.EventDispatcher) error {
	conn, serviceID, err := dialWebsocket(ctx, endpointURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	c.logger.Info(ctx, fmt.Sprintf("feishu websocket connected service_id=%d", serviceID))
	session := &wsSession{
		client:     c,
		conn:       conn,
		serviceID:  serviceID,
		config:     config,
		dispatcher: dispatcher,
	}
	return session.run(ctx)
}

func (s *wsSession) run(ctx context.Context) error {
	done := make(chan struct{})
	errCh := make(chan error, 1)

	go func() {
		defer close(done)
		errCh <- s.readLoop(ctx)
	}()
	go s.pingLoop(ctx, done)

	select {
	case err := <-errCh:
		if ctx.Err() != nil {
			return nil
		}
		return err
	case <-ctx.Done():
		_ = s.conn.Close()
		<-done
		return nil
	}
}

func (s *wsSession) readLoop(ctx context.Context) error {
	fragments := newWSFragmentAccumulator()
	for {
		messageType, message, err := s.conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) || ws.IsCloseError(err, ws.CloseNormalClosure, ws.CloseGoingAway) {
				return nil
			}
			return fmt.Errorf("read websocket message: %w", err)
		}
		if messageType != ws.BinaryMessage {
			s.client.logger.Warn(ctx, fmt.Sprintf("feishu websocket ignore non-binary message type=%d", messageType))
			continue
		}
		if err := s.handleFrame(ctx, message, fragments); err != nil {
			return err
		}
	}
}

func (s *wsSession) pingLoop(ctx context.Context, done <-chan struct{}) {
	for {
		timer := time.NewTimer(s.config.pingInterval())
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-done:
			timer.Stop()
			return
		case <-timer.C:
			frame := larkws.NewPingFrame(s.serviceID)
			payload, err := frame.Marshal()
			if err != nil {
				s.client.logger.Warn(ctx, fmt.Sprintf("feishu websocket ping marshal failed: %v", err))
				continue
			}
			if err := s.writeMessage(ws.BinaryMessage, payload); err != nil {
				if ctx.Err() == nil {
					s.client.logger.Warn(ctx, fmt.Sprintf("feishu websocket ping failed: %v", err))
				}
			}
		}
	}
}

func (s *wsSession) handleFrame(ctx context.Context, raw []byte, fragments *wsFragmentAccumulator) error {
	var frame larkws.Frame
	if err := frame.Unmarshal(raw); err != nil {
		s.client.logger.Error(ctx, fmt.Sprintf("feishu websocket frame decode failed: %v", err))
		return nil
	}

	switch larkws.FrameType(frame.Method) {
	case larkws.FrameTypeControl:
		s.handleControlFrame(ctx, frame)
	case larkws.FrameTypeData:
		return s.handleDataFrame(ctx, frame, fragments)
	}
	return nil
}

func (s *wsSession) handleControlFrame(ctx context.Context, frame larkws.Frame) {
	headers := larkws.Headers(frame.Headers)
	if larkws.MessageType(headers.GetString(larkws.HeaderType)) != larkws.MessageTypePong {
		return
	}
	if len(frame.Payload) == 0 {
		return
	}
	var config larkws.ClientConfig
	if err := json.Unmarshal(frame.Payload, &config); err != nil {
		s.client.logger.Warn(ctx, fmt.Sprintf("feishu websocket pong decode failed: %v", err))
		return
	}
	s.config.apply(&config)
}

func (s *wsSession) handleDataFrame(ctx context.Context, frame larkws.Frame, fragments *wsFragmentAccumulator) error {
	headers := larkws.Headers(frame.Headers)
	sum := headers.GetInt(larkws.HeaderSum)
	seq := headers.GetInt(larkws.HeaderSeq)
	messageID := headers.GetString(larkws.HeaderMessageID)
	traceID := headers.GetString(larkws.HeaderTraceID)
	messageType := larkws.MessageType(headers.GetString(larkws.HeaderType))

	payload := frame.Payload
	if combined, ok := fragments.Combine(messageID, sum, seq, payload); sum <= 1 || ok {
		payload = combined
	} else {
		return nil
	}

	switch messageType {
	case larkws.MessageTypeEvent:
	case larkws.MessageTypeCard:
		return nil
	default:
		return nil
	}

	started := time.Now()
	respPayload, err := s.dispatcher.Do(ctx, payload)
	headers.Add(larkws.HeaderBizRt, strconv.FormatInt(time.Since(started).Milliseconds(), 10))

	response := larkws.NewResponseByCode(http.StatusOK)
	if err != nil {
		s.client.logger.Error(ctx, fmt.Sprintf("feishu websocket handler failed type=%s message_id=%s trace_id=%s err=%v", messageType, messageID, traceID, err))
		response = larkws.NewResponseByCode(http.StatusInternalServerError)
	} else if respPayload != nil {
		data, marshalErr := json.Marshal(respPayload)
		if marshalErr != nil {
			s.client.logger.Error(ctx, fmt.Sprintf("feishu websocket response encode failed type=%s message_id=%s trace_id=%s err=%v", messageType, messageID, traceID, marshalErr))
			response = larkws.NewResponseByCode(http.StatusInternalServerError)
		} else {
			response.Data = data
		}
	}

	data, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("encode websocket response: %w", err)
	}
	frame.Payload = data
	frame.Headers = headers
	packet, err := frame.Marshal()
	if err != nil {
		return fmt.Errorf("encode websocket frame: %w", err)
	}
	if err := s.writeMessage(ws.BinaryMessage, packet); err != nil {
		return fmt.Errorf("write websocket response: %w", err)
	}
	return nil
}

func (s *wsSession) writeMessage(messageType int, data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteMessage(messageType, data)
}

func (c *Client) fetchWebsocketEndpoint(ctx context.Context) (string, *larkws.ClientConfig, error) {
	body, err := json.Marshal(map[string]string{
		"AppID":     c.appID,
		"AppSecret": c.appSecret,
	})
	if err != nil {
		return "", nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, lark.FeishuBaseUrl+larkws.GenEndpointUri, bytes.NewReader(body))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("locale", "zh")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil, larkws.NewServerError(resp.StatusCode, "system busy")
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}
	var endpointResp larkws.EndpointResp
	if err := json.Unmarshal(data, &endpointResp); err != nil {
		return "", nil, err
	}

	switch endpointResp.Code {
	case larkws.OK:
	case larkws.SystemBusy:
		return "", nil, larkws.NewServerError(endpointResp.Code, "system busy")
	case larkws.InternalError:
		return "", nil, larkws.NewServerError(endpointResp.Code, endpointResp.Msg)
	default:
		return "", nil, larkws.NewClientError(endpointResp.Code, endpointResp.Msg)
	}

	if endpointResp.Data == nil || endpointResp.Data.Url == "" {
		return "", nil, larkws.NewServerError(http.StatusInternalServerError, "endpoint is null")
	}
	return endpointResp.Data.Url, endpointResp.Data.ClientConfig, nil
}

func dialWebsocket(ctx context.Context, endpointURL string) (*ws.Conn, int32, error) {
	parsed, err := url.Parse(endpointURL)
	if err != nil {
		return nil, 0, err
	}
	serviceID, err := strconv.ParseInt(parsed.Query().Get(larkws.ServiceID), 10, 32)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid websocket service id: %w", err)
	}

	conn, resp, err := ws.DefaultDialer.DialContext(ctx, endpointURL, nil)
	if err != nil {
		if resp == nil {
			return nil, 0, err
		}
		defer resp.Body.Close()
		return nil, 0, parseHandshakeError(resp)
	}
	if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
		defer resp.Body.Close()
		conn.Close()
		return nil, 0, parseHandshakeError(resp)
	}
	return conn, int32(serviceID), nil
}

func parseHandshakeError(resp *http.Response) error {
	code, _ := strconv.Atoi(resp.Header.Get(larkws.HeaderHandshakeStatus))
	msg := resp.Header.Get(larkws.HeaderHandshakeMsg)
	switch code {
	case larkws.AuthFailed:
		authCode, _ := strconv.Atoi(resp.Header.Get(larkws.HeaderHandshakeAuthErrCode))
		if authCode == larkws.ExceedConnLimit {
			return larkws.NewClientError(code, msg)
		}
		return larkws.NewServerError(code, msg)
	case larkws.Forbidden:
		return larkws.NewClientError(code, msg)
	default:
		return larkws.NewServerError(code, msg)
	}
}

func isFeishuClientError(err error) bool {
	var clientErr *larkws.ClientError
	return errors.As(err, &clientErr)
}
