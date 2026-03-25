package weixin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultBaseURL         = "https://ilinkai.weixin.qq.com"
	defaultCDNBaseURL      = "https://novac2c.cdn.weixin.qq.com/c2c"
	defaultLongPollTimeout = 35 * time.Second
	defaultAPITimeout      = 15 * time.Second
	maxIlinkHTTPBody       = 64 << 20
	channelVersion         = "tmux-connect-weixin/1.0"
	maxWeixinChunk         = 3800
	oldMessageThreshold    = 5 * time.Minute
)

const (
	runtimeStateScopeCursor       = "cursor"
	runtimeStateScopeContextToken = "context_token"
)

type RuntimeStateStore interface {
	GetPlatformRuntimeState(context.Context, string, string, string) (string, error)
	SetPlatformRuntimeState(context.Context, string, string, string, string) error
}

type ClientConfig struct {
	Token      string
	BaseURL    string
	CDNBaseURL string
	RouteTag   string
	HTTPClient *http.Client
	Stderr     io.Writer
	Store      RuntimeStateStore
}

type MessageEvent struct {
	ChatID    string
	SenderID  string
	MessageID string
	Text      string
}

type Client struct {
	api        *apiClient
	httpClient *http.Client
	cdnBaseURL string
	stderr     io.Writer
	store      RuntimeStateStore

	dedupMu sync.Mutex
	dedup   map[string]time.Time
}

type apiClient struct {
	baseURL    string
	token      string
	routeTag   string
	httpClient *http.Client
}

func NewClient(cfg ClientConfig) (*Client, error) {
	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		return nil, fmt.Errorf("weixin token is required")
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultAPITimeout}
	}
	return &Client{
		api: &apiClient{
			baseURL:    normalizeBaseURL(cfg.BaseURL, defaultBaseURL),
			token:      token,
			routeTag:   strings.TrimSpace(cfg.RouteTag),
			httpClient: httpClient,
		},
		httpClient: httpClient,
		cdnBaseURL: normalizeBaseURL(cfg.CDNBaseURL, defaultCDNBaseURL),
		stderr:     cfg.Stderr,
		store:      cfg.Store,
		dedup:      make(map[string]time.Time),
	}, nil
}

func normalizeBaseURL(value string, fallback string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" {
		value = strings.TrimRight(strings.TrimSpace(fallback), "/")
	}
	return value
}

func (c *Client) Close() error { return nil }

func (c *Client) Run(ctx context.Context, handler func(context.Context, MessageEvent) error) error {
	cursor, err := c.runtimeState(ctx, runtimeStateScopeCursor, "")
	if err != nil {
		return err
	}
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		resp, err := c.api.getUpdates(ctx, cursor)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			c.logf("weixin getUpdates error: %v", err)
			if err := waitForContextOrTimeout(ctx, backoff); err != nil {
				return err
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second

		if resp.Errcode == sessionExpiredErrcode {
			c.logf("weixin session expired: errcode=%d errmsg=%s", resp.Errcode, strings.TrimSpace(resp.Errmsg))
			if err := waitForContextOrTimeout(ctx, time.Minute); err != nil {
				return err
			}
			continue
		}
		if resp.GetUpdatesBuf != "" && resp.GetUpdatesBuf != cursor {
			cursor = resp.GetUpdatesBuf
			if err := c.setRuntimeState(ctx, runtimeStateScopeCursor, "", cursor); err != nil {
				return err
			}
		}
		for i := range resp.Msgs {
			event, ok, err := c.toMessageEvent(ctx, &resp.Msgs[i])
			if err != nil {
				c.logf("weixin inbound parse error: %v", err)
				continue
			}
			if !ok {
				continue
			}
			if err := handler(ctx, event); err != nil {
				return err
			}
		}
	}
}

func (c *Client) SendText(ctx context.Context, chatID string, text string) (string, error) {
	chatID = strings.TrimSpace(chatID)
	text = strings.TrimSpace(text)
	if chatID == "" {
		return "", fmt.Errorf("weixin chat id is required")
	}
	if text == "" {
		return "", fmt.Errorf("weixin text is required")
	}
	contextToken, err := c.contextToken(ctx, chatID)
	if err != nil {
		return "", err
	}
	var messageID string
	for _, chunk := range splitUTF8(text, maxWeixinChunk) {
		messageID = "tmuxconn-" + randomHex(8)
		if err := c.api.sendItems(ctx, chatID, contextToken, messageID, []messageItem{{
			Type:     messageItemText,
			TextItem: &textItem{Text: chunk},
		}}); err != nil {
			return "", err
		}
	}
	return messageID, nil
}

func (c *Client) SendImage(ctx context.Context, chatID string, fileName string, data []byte, caption string) (string, error) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return "", fmt.Errorf("weixin chat id is required")
	}
	if len(data) == 0 {
		return "", fmt.Errorf("weixin image data is required")
	}
	contextToken, err := c.contextToken(ctx, chatID)
	if err != nil {
		return "", err
	}
	aesKey := make([]byte, 16)
	if _, err := rand.Read(aesKey); err != nil {
		return "", fmt.Errorf("weixin image aes key: %w", err)
	}
	filekey := randomHex(16)
	req := getUploadURLRequest{
		Filekey:     filekey,
		MediaType:   uploadMediaImage,
		ToUserID:    chatID,
		Rawsize:     len(data),
		Rawfilemd5:  md5Hex(data),
		Filesize:    aesECBPaddedSize(len(data)),
		NoNeedThumb: true,
		Aeskey:      hex.EncodeToString(aesKey),
	}
	resp, err := c.api.getUploadURL(ctx, req)
	if err != nil {
		return "", err
	}
	downloadParam, err := uploadBufferToCDN(ctx, c.httpClient, c.cdnBaseURL, resp.UploadParam, filekey, data, aesKey, "send image")
	if err != nil {
		return "", err
	}
	items := []messageItem{{
		Type: messageItemImage,
		ImageItem: &imageItem{
			Media: &cdnMedia{
				EncryptQueryParam: downloadParam,
				AESKey:            base64.StdEncoding.EncodeToString(aesKey),
				EncryptType:       1,
			},
			MidSize: aesECBPaddedSize(len(data)),
		},
	}}
	messageID := "tmuxconn-" + randomHex(8)
	if err := c.api.sendItems(ctx, chatID, contextToken, messageID, items); err != nil {
		return "", err
	}
	caption = strings.TrimSpace(caption)
	if caption != "" {
		if _, err := c.SendText(ctx, chatID, caption); err != nil {
			return "", err
		}
	}
	return messageID, nil
}

func (c *Client) contextToken(ctx context.Context, chatID string) (string, error) {
	token, err := c.runtimeState(ctx, runtimeStateScopeContextToken, chatID)
	if err != nil {
		return "", err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("weixin context_token missing for %q; the user must message the bot first", chatID)
	}
	return token, nil
}

func (c *Client) toMessageEvent(ctx context.Context, msg *weixinMessage) (MessageEvent, bool, error) {
	if msg == nil {
		return MessageEvent{}, false, nil
	}
	if msg.MessageType == messageTypeBot || (msg.MessageType != 0 && msg.MessageType != messageTypeUser) {
		return MessageEvent{}, false, nil
	}
	fromUserID := strings.TrimSpace(msg.FromUserID)
	if fromUserID == "" {
		return MessageEvent{}, false, nil
	}
	if msg.CreateTimeMs > 0 && time.Since(time.UnixMilli(msg.CreateTimeMs)) > oldMessageThreshold {
		return MessageEvent{}, false, nil
	}
	dedupKey := fmt.Sprintf("%s|%d|%d|%d|%s", fromUserID, msg.MessageID, msg.Seq, msg.CreateTimeMs, strings.TrimSpace(msg.ClientID))
	if c.isDuplicate(dedupKey) {
		return MessageEvent{}, false, nil
	}
	if token := strings.TrimSpace(msg.ContextToken); token != "" {
		if err := c.setRuntimeState(ctx, runtimeStateScopeContextToken, fromUserID, token); err != nil {
			return MessageEvent{}, false, err
		}
	}
	text := strings.TrimSpace(bodyFromItemList(msg.ItemList))
	if text == "" {
		return MessageEvent{}, false, nil
	}
	messageID := strings.TrimSpace(fmt.Sprintf("%d", msg.MessageID))
	if messageID == "0" || messageID == "" {
		messageID = "in-" + randomHex(8)
	}
	return MessageEvent{
		ChatID:    fromUserID,
		SenderID:  fromUserID,
		MessageID: messageID,
		Text:      text,
	}, true, nil
}

func (c *Client) isDuplicate(key string) bool {
	c.dedupMu.Lock()
	defer c.dedupMu.Unlock()
	now := time.Now()
	for existing, ts := range c.dedup {
		if now.Sub(ts) > oldMessageThreshold {
			delete(c.dedup, existing)
		}
	}
	if _, ok := c.dedup[key]; ok {
		return true
	}
	c.dedup[key] = now
	return false
}

func (c *Client) runtimeState(ctx context.Context, scope string, entityID string) (string, error) {
	if c.store == nil {
		return "", nil
	}
	value, err := c.store.GetPlatformRuntimeState(ctx, "weixin", scope, entityID)
	if err != nil {
		return "", fmt.Errorf("get weixin runtime state %s/%s: %w", scope, entityID, err)
	}
	return value, nil
}

func (c *Client) setRuntimeState(ctx context.Context, scope string, entityID string, value string) error {
	if c.store == nil {
		return nil
	}
	if err := c.store.SetPlatformRuntimeState(ctx, "weixin", scope, entityID, value); err != nil {
		return fmt.Errorf("set weixin runtime state %s/%s: %w", scope, entityID, err)
	}
	return nil
}

func (c *Client) logf(format string, args ...any) {
	if c.stderr == nil {
		return
	}
	fmt.Fprintf(c.stderr, format+"\n", args...)
}

func randomWechatUIN() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return base64.StdEncoding.EncodeToString([]byte("0000"))
	}
	u := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", u)))
}

func (c *apiClient) longPollClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultLongPollTimeout
	}
	baseClient := c.httpClient
	if baseClient == nil {
		baseClient = &http.Client{Timeout: defaultAPITimeout}
	}
	transport := baseClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	if typed, ok := transport.(*http.Transport); ok {
		transport = typed.Clone()
	}
	return &http.Client{
		Timeout:   timeout + 5*time.Second,
		Transport: transport,
	}
}

func (c *apiClient) post(ctx context.Context, endpoint string, body []byte, timeout time.Duration, label string) ([]byte, error) {
	url := c.baseURL + "/" + strings.TrimPrefix(endpoint, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("weixin %s: new request: %w", label, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("AuthorizationType", "ilink_bot_token")
	req.Header.Set("X-WECHAT-UIN", randomWechatUIN())
	req.Header.Set("Authorization", "Bearer "+c.token)
	if c.routeTag != "" {
		req.Header.Set("SKRouteTag", c.routeTag)
	}
	client := c.httpClient
	if timeout > 0 {
		client = c.longPollClient(timeout)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("weixin %s: %w", label, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxIlinkHTTPBody+1))
	if err != nil {
		return nil, fmt.Errorf("weixin %s: read body: %w", label, err)
	}
	if len(raw) > maxIlinkHTTPBody {
		return nil, fmt.Errorf("weixin %s: response body exceeds %d bytes", label, maxIlinkHTTPBody)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weixin %s: http %d: %s", label, resp.StatusCode, truncateForLog(raw, 256))
	}
	return raw, nil
}

func (c *apiClient) getUpdates(ctx context.Context, cursor string) (*getUpdatesResp, error) {
	payload, err := json.Marshal(getUpdatesReq{
		GetUpdatesBuf: cursor,
		BaseInfo:      baseInfo{ChannelVersion: channelVersion},
	})
	if err != nil {
		return nil, err
	}
	raw, err := c.post(ctx, "/ilink/bot/getupdates", payload, defaultLongPollTimeout, "getupdates")
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return &getUpdatesResp{Ret: 0, GetUpdatesBuf: cursor}, nil
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return &getUpdatesResp{Ret: 0, GetUpdatesBuf: cursor}, nil
		}
		return nil, err
	}
	var resp getUpdatesResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("weixin getupdates json: %w", err)
	}
	if resp.Errcode == sessionExpiredErrcode {
		return &resp, nil
	}
	if resp.Ret != 0 || resp.Errcode != 0 {
		return nil, fmt.Errorf("weixin getupdates: ret=%d errcode=%d errmsg=%s", resp.Ret, resp.Errcode, strings.TrimSpace(resp.Errmsg))
	}
	return &resp, nil
}

func (c *apiClient) sendItems(ctx context.Context, toUserID string, contextToken string, clientID string, items []messageItem) error {
	req := sendMessageReq{
		Msg: weixinOutboundMsg{
			ToUserID:     toUserID,
			ClientID:     clientID,
			MessageType:  messageTypeBot,
			MessageState: messageStateFinish,
			ItemList:     items,
			ContextToken: contextToken,
		},
		BaseInfo: baseInfo{ChannelVersion: channelVersion},
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}
	raw, err := c.post(ctx, "/ilink/bot/sendmessage", payload, 0, "sendmessage")
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var resp sendMessageResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("weixin sendmessage json: %w", err)
	}
	if resp.Ret != 0 {
		return fmt.Errorf("weixin sendmessage: ret=%d errcode=%d errmsg=%s", resp.Ret, resp.Errcode, strings.TrimSpace(resp.Errmsg))
	}
	return nil
}

func (c *apiClient) getUploadURL(ctx context.Context, req getUploadURLRequest) (*getUploadURLResponse, error) {
	req.BaseInfo = baseInfo{ChannelVersion: channelVersion}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	raw, err := c.post(ctx, "/ilink/bot/getuploadurl", payload, 0, "getuploadurl")
	if err != nil {
		return nil, err
	}
	var resp getUploadURLResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("weixin getuploadurl json: %w", err)
	}
	if strings.TrimSpace(resp.UploadParam) == "" {
		return nil, fmt.Errorf("weixin getuploadurl: empty upload_param")
	}
	return &resp, nil
}

func truncateForLog(raw []byte, max int) string {
	if len(raw) <= max {
		return string(raw)
	}
	return string(raw[:max]) + "..."
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func waitForContextOrTimeout(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
