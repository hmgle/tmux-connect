package weixin

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const maxWeixinMediaBytes = 100 << 20

var hex32RE = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)

func aesECBPaddedSize(plaintextLen int) int {
	if plaintextLen < 0 {
		return 0
	}
	return ((plaintextLen + aes.BlockSize) / aes.BlockSize) * aes.BlockSize
}

func pkcs7Pad(b []byte, blockSize int) []byte {
	n := blockSize - (len(b) % blockSize)
	return append(b, bytes.Repeat([]byte{byte(n)}, n)...)
}

func pkcs7Unpad(b []byte, blockSize int) ([]byte, error) {
	if len(b) == 0 || len(b)%blockSize != 0 {
		return nil, fmt.Errorf("invalid padded length %d", len(b))
	}
	n := int(b[len(b)-1])
	if n == 0 || n > blockSize || n > len(b) {
		return nil, fmt.Errorf("invalid pkcs7 padding")
	}
	for i := len(b) - n; i < len(b); i++ {
		if b[i] != byte(n) {
			return nil, fmt.Errorf("invalid pkcs7 padding")
		}
	}
	return b[:len(b)-n], nil
}

func encryptAESECB(plaintext []byte, key []byte) ([]byte, error) {
	if len(key) != 16 {
		return nil, fmt.Errorf("aes key must be 16 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	out := make([]byte, len(padded))
	for i := 0; i < len(padded); i += aes.BlockSize {
		block.Encrypt(out[i:i+aes.BlockSize], padded[i:i+aes.BlockSize])
	}
	return out, nil
}

func parseAesKey(aesKeyBase64 string, label string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(aesKeyBase64))
	if err != nil {
		return nil, fmt.Errorf("%s: aes_key base64: %w", label, err)
	}
	if len(decoded) == 16 {
		return decoded, nil
	}
	if len(decoded) == 32 && hex32RE.Match(decoded) {
		return hex.DecodeString(string(decoded))
	}
	return nil, fmt.Errorf("%s: invalid aes_key length %d", label, len(decoded))
}

func buildCdnUploadURL(cdnBase string, uploadParam string, filekey string) string {
	return fmt.Sprintf("%s/upload?encrypted_query_param=%s&filekey=%s",
		strings.TrimRight(cdnBase, "/"),
		url.QueryEscape(uploadParam),
		url.QueryEscape(filekey))
}

func uploadBufferToCDN(ctx context.Context, client *http.Client, cdnBase string, uploadParam string, filekey string, plaintext []byte, aesKey []byte, label string) (string, error) {
	ciphertext, err := encryptAESECB(plaintext, aesKey)
	if err != nil {
		return "", fmt.Errorf("%s: encrypt: %w", label, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, buildCdnUploadURL(cdnBase, uploadParam, filekey), bytes.NewReader(ciphertext))
	if err != nil {
		return "", fmt.Errorf("%s: new request: %w", label, err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s: upload: %w", label, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: http %d", label, resp.StatusCode)
	}
	downloadParam := strings.TrimSpace(resp.Header.Get("x-encrypted-param"))
	if downloadParam == "" {
		return "", fmt.Errorf("%s: missing x-encrypted-param", label)
	}
	return downloadParam, nil
}

func md5Hex(b []byte) string {
	sum := md5.Sum(b)
	return hex.EncodeToString(sum[:])
}
