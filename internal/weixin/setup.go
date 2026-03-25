package weixin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultBotType  = "3"
	qrPollTimeout   = 35 * time.Second
	maxQRBodyBytes  = 1 << 20
	setupAPIVersion = "tmux-connect-weixin-setup/1.0"
)

type BotQRCode struct {
	QRCode           string `json:"qrcode"`
	QRCodeImgContent string `json:"qrcode_img_content"`
}

type QRCodeStatus struct {
	Status      string `json:"status"`
	BotToken    string `json:"bot_token"`
	IlinkBotID  string `json:"ilink_bot_id"`
	BaseURL     string `json:"baseurl"`
	IlinkUserID string `json:"ilink_user_id"`
}

func FetchBotQRCode(ctx context.Context, httpClient *http.Client, apiBase string, botType string, routeTag string) (*BotQRCode, error) {
	baseURL := normalizeBaseURL(apiBase, defaultBaseURL)
	if strings.TrimSpace(botType) == "" {
		botType = DefaultBotType
	}
	u, err := url.Parse(baseURL + "/")
	if err != nil {
		return nil, err
	}
	u = u.JoinPath("ilink", "bot", "get_bot_qrcode")
	query := u.Query()
	query.Set("bot_type", strings.TrimSpace(botType))
	u.RawQuery = query.Encode()

	raw, err := doSetupGET(ctx, httpClient, u.String(), routeTag)
	if err != nil {
		return nil, fmt.Errorf("get_bot_qrcode: %w", err)
	}
	var out BotQRCode
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("get_bot_qrcode json: %w", err)
	}
	return &out, nil
}

func PollQRCodeStatus(ctx context.Context, httpClient *http.Client, apiBase string, qrCode string, routeTag string) (*QRCodeStatus, error) {
	baseURL := normalizeBaseURL(apiBase, defaultBaseURL)
	u, err := url.Parse(baseURL + "/")
	if err != nil {
		return nil, err
	}
	u = u.JoinPath("ilink", "bot", "get_qrcode_status")
	query := u.Query()
	query.Set("qrcode", strings.TrimSpace(qrCode))
	u.RawQuery = query.Encode()

	pollCtx, cancel := context.WithTimeout(ctx, qrPollTimeout+2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(pollCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("iLink-App-ClientVersion", "1")
	if routeTag = strings.TrimSpace(routeTag); routeTag != "" {
		req.Header.Set("SKRouteTag", routeTag)
	}
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: qrPollTimeout + 5*time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return &QRCodeStatus{Status: "wait"}, nil
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return &QRCodeStatus{Status: "wait"}, nil
		}
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxQRBodyBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get_qrcode_status http %d: %s", resp.StatusCode, truncateForLog(body, 256))
	}
	var out QRCodeStatus
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("get_qrcode_status json: %w", err)
	}
	return &out, nil
}

func VerifyToken(ctx context.Context, httpClient *http.Client, apiBase string, token string, routeTag string) error {
	payload := []byte(`{"get_updates_buf":"","base_info":{"channel_version":"` + setupAPIVersion + `"}}`)
	api := &apiClient{
		baseURL:    normalizeBaseURL(apiBase, defaultBaseURL),
		token:      strings.TrimSpace(token),
		routeTag:   strings.TrimSpace(routeTag),
		httpClient: httpClient,
	}
	raw, err := api.post(ctx, "/ilink/bot/getupdates", payload, 15*time.Second, "verify token")
	if err != nil {
		return err
	}
	var resp getUpdatesResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("verify token: invalid json response: %w", err)
	}
	// Verification is intentionally stricter than runtime polling: an expired
	// session means this token is not usable for setup/bind purposes.
	if resp.Ret != 0 || resp.Errcode != 0 {
		return fmt.Errorf("verify token: ret=%d errcode=%d errmsg=%s", resp.Ret, resp.Errcode, strings.TrimSpace(resp.Errmsg))
	}
	return nil
}

func doSetupGET(ctx context.Context, httpClient *http.Client, fullURL string, routeTag string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	if routeTag = strings.TrimSpace(routeTag); routeTag != "" {
		req.Header.Set("SKRouteTag", routeTag)
	}
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: qrPollTimeout + 5*time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxQRBodyBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, truncateForLog(body, 256))
	}
	return bytes.TrimSpace(body), nil
}
