package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (c *Client) call(ctx context.Context, method string, payload any, dest any) error {
	if strings.TrimSpace(c.token) == "" {
		return fmt.Errorf("telegram token is required")
	}
	method = strings.TrimSpace(method)
	if method == "" {
		return fmt.Errorf("telegram method is required")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal telegram request: %w", err)
	}

	endpoint, err := url.JoinPath(c.baseURL, "bot"+c.token, method)
	if err != nil {
		return fmt.Errorf("build telegram url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	return c.do(req, method, dest)
}

func (c *Client) callMultipart(ctx context.Context, method string, body *bytes.Buffer, contentType string, dest any) error {
	if strings.TrimSpace(c.token) == "" {
		return fmt.Errorf("telegram token is required")
	}
	method = strings.TrimSpace(method)
	if method == "" {
		return fmt.Errorf("telegram method is required")
	}

	endpoint, err := url.JoinPath(c.baseURL, "bot"+c.token, method)
	if err != nil {
		return fmt.Errorf("build telegram url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body.Bytes()))
	if err != nil {
		return fmt.Errorf("build telegram request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	return c.do(req, method, dest)
}

func (c *Client) do(req *http.Request, method string, dest any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("telegram %s: %w", method, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return fmt.Errorf("read telegram %s response: %w", method, err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("telegram %s: http %d: %s", method, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("decode telegram %s response: %w", method, err)
	}

	switch value := dest.(type) {
	case *apiResponse[[]Update]:
		if !value.OK {
			return fmt.Errorf("telegram %s failed: %s", method, strings.TrimSpace(value.Description))
		}
	case *apiResponse[Message]:
		if !value.OK {
			return fmt.Errorf("telegram %s failed: %s", method, strings.TrimSpace(value.Description))
		}
	case *apiResponse[bool]:
		if !value.OK {
			return fmt.Errorf("telegram %s failed: %s", method, strings.TrimSpace(value.Description))
		}
	default:
		return nil
	}
	return nil
}
