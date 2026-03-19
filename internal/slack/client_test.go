package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"

	slackapi "github.com/slack-go/slack"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func httpResponse(status int, body string, contentType string) *http.Response {
	if contentType == "" {
		contentType = "application/json"
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func parseURLEncodedBody(t *testing.T, req *http.Request) url.Values {
	t.Helper()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	return values
}

func parseMultipartFile(t *testing.T, req *http.Request) []byte {
	t.Helper()

	contentType := req.Header.Get("Content-Type")
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("ParseMediaType() error = %v", err)
	}
	reader := multipart.NewReader(req.Body, params["boundary"])
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextPart() error = %v", err)
		}
		if part.FormName() != "file" {
			continue
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("ReadAll(part) error = %v", err)
		}
		return data
	}
	t.Fatal("multipart body missing file part")
	return nil
}

func TestUploadImageUsesFileUploadV2(t *testing.T) {
	t.Parallel()

	var (
		gotUploadURLValues url.Values
		gotCompleteValues  url.Values
		gotUploadBody      []byte
		filesUploadCalled  bool
	)

	httpClient := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/files.getUploadURLExternal":
			gotUploadURLValues = parseURLEncodedBody(t, req)
			return httpResponse(http.StatusOK, `{"ok":true,"upload_url":"https://upload.slack.test/upload","file_id":"F123"}`, ""), nil
		case "/upload":
			gotUploadBody = parseMultipartFile(t, req)
			return httpResponse(http.StatusOK, "ok", "text/plain"), nil
		case "/files.completeUploadExternal":
			gotCompleteValues = parseURLEncodedBody(t, req)
			return httpResponse(http.StatusOK, `{"ok":true,"files":[{"id":"F123","title":"default:%5 snapshot"}]}`, ""), nil
		case "/files.upload":
			filesUploadCalled = true
			return httpResponse(http.StatusGone, `{"ok":false,"error":"gone"}`, ""), nil
		default:
			t.Fatalf("unexpected path %q", req.URL.Path)
			return nil, nil
		}
	})

	client := NewClient(
		"xoxb-test",
		"xapp-test",
		slackapi.OptionAPIURL("https://api.slack.test/"),
		slackapi.OptionHTTPClient(httpClient),
	)
	fileID, err := client.UploadImage(context.Background(), "C123", "1700000000.000001", "pane-snapshot.png", []byte("pngbytes"), "default:%5 snapshot")
	if err != nil {
		t.Fatalf("UploadImage() error = %v", err)
	}
	if fileID != "F123" {
		t.Fatalf("UploadImage() id = %q, want F123", fileID)
	}
	if filesUploadCalled {
		t.Fatal("UploadImage() used deprecated files.upload endpoint")
	}
	if got := gotUploadURLValues.Get("filename"); got != "pane-snapshot.png" {
		t.Fatalf("files.getUploadURLExternal filename = %q, want pane-snapshot.png", got)
	}
	if got := gotUploadURLValues.Get("length"); got != "8" {
		t.Fatalf("files.getUploadURLExternal length = %q, want 8", got)
	}
	if got := gotCompleteValues.Get("channel_id"); got != "C123" {
		t.Fatalf("files.completeUploadExternal channel_id = %q, want C123", got)
	}
	if got := gotCompleteValues.Get("thread_ts"); got != "1700000000.000001" {
		t.Fatalf("files.completeUploadExternal thread_ts = %q, want thread timestamp", got)
	}
	if got := gotCompleteValues.Get("initial_comment"); got != "default:%5 snapshot" {
		t.Fatalf("files.completeUploadExternal initial_comment = %q, want caption", got)
	}
	if got := string(gotUploadBody); got != "pngbytes" {
		t.Fatalf("uploaded bytes = %q, want pngbytes", got)
	}
}

func TestAnnotateUploadImageErrorAddsSlackScopeHint(t *testing.T) {
	t.Parallel()

	err := annotateUploadImageError(slackapi.SlackErrorResponse{Err: "missing_scope"})
	if err == nil {
		t.Fatal("annotateUploadImageError() error = nil")
	}
	if !strings.Contains(err.Error(), "files:write") {
		t.Fatalf("annotateUploadImageError() = %q, want files:write hint", err)
	}
}

func TestAnnotateUploadImageErrorLeavesOtherErrorsUntouched(t *testing.T) {
	t.Parallel()

	baseErr := errors.New("boom")
	err := annotateUploadImageError(baseErr)
	if !errors.Is(err, baseErr) {
		t.Fatalf("annotateUploadImageError() = %v, want wrapped boom", err)
	}
	if err.Error() != "boom" {
		t.Fatalf("annotateUploadImageError() = %q, want boom", err)
	}
}

func TestUploadImageRejectsEmptyData(t *testing.T) {
	t.Parallel()

	client := NewClient("xoxb-test", "xapp-test")
	_, err := client.UploadImage(context.Background(), "C123", "", "snapshot.png", nil, "caption")
	if err == nil {
		t.Fatal("UploadImage() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "requires data") {
		t.Fatalf("UploadImage() error = %q, want data hint", err)
	}
}

func TestUploadImageCompletePayloadIncludesFileID(t *testing.T) {
	t.Parallel()

	var gotFilesField string

	httpClient := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/files.getUploadURLExternal":
			return httpResponse(http.StatusOK, `{"ok":true,"upload_url":"https://upload.slack.test/upload","file_id":"F999"}`, ""), nil
		case "/upload":
			_, _ = io.Copy(io.Discard, req.Body)
			return httpResponse(http.StatusOK, "ok", "text/plain"), nil
		case "/files.completeUploadExternal":
			values := parseURLEncodedBody(t, req)
			gotFilesField = values.Get("files")
			return httpResponse(http.StatusOK, `{"ok":true,"files":[{"id":"F999","title":"pane snapshot"}]}`, ""), nil
		default:
			t.Fatalf("unexpected path %q", req.URL.Path)
			return nil, nil
		}
	})

	client := NewClient(
		"xoxb-test",
		"xapp-test",
		slackapi.OptionAPIURL("https://api.slack.test/"),
		slackapi.OptionHTTPClient(httpClient),
	)
	if _, err := client.UploadImage(context.Background(), "C123", "", "snapshot.png", []byte("png"), "pane snapshot"); err != nil {
		t.Fatalf("UploadImage() error = %v", err)
	}

	var files []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(gotFilesField), &files); err != nil {
		t.Fatalf("Unmarshal(files field) error = %v", err)
	}
	if len(files) != 1 || files[0].ID != "F999" {
		t.Fatalf("files field = %#v, want file id F999", files)
	}
}

func TestUploadImageDefaultsFileName(t *testing.T) {
	t.Parallel()

	var gotFileName string

	httpClient := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/files.getUploadURLExternal":
			values := parseURLEncodedBody(t, req)
			gotFileName = values.Get("filename")
			return httpResponse(http.StatusOK, `{"ok":true,"upload_url":"https://upload.slack.test/upload","file_id":"F111"}`, ""), nil
		case "/upload":
			_, _ = io.Copy(io.Discard, req.Body)
			return httpResponse(http.StatusOK, "ok", "text/plain"), nil
		case "/files.completeUploadExternal":
			return httpResponse(http.StatusOK, `{"ok":true,"files":[{"id":"F111","title":"pane snapshot"}]}`, ""), nil
		default:
			t.Fatalf("unexpected path %q", req.URL.Path)
			return nil, nil
		}
	})

	client := NewClient(
		"xoxb-test",
		"xapp-test",
		slackapi.OptionAPIURL("https://api.slack.test/"),
		slackapi.OptionHTTPClient(httpClient),
	)
	if _, err := client.UploadImage(context.Background(), "C123", "", "", []byte("png"), "pane snapshot"); err != nil {
		t.Fatalf("UploadImage() error = %v", err)
	}
	if gotFileName != "snapshot.png" {
		t.Fatalf("default filename = %q, want snapshot.png", gotFileName)
	}
}

func TestUploadImageScopeErrorsCarryHint(t *testing.T) {
	t.Parallel()

	httpClient := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/files.getUploadURLExternal" {
			t.Fatalf("unexpected path %q", req.URL.Path)
		}
		return httpResponse(http.StatusOK, `{"ok":false,"error":"missing_scope"}`, ""), nil
	})

	client := NewClient(
		"xoxb-test",
		"xapp-test",
		slackapi.OptionAPIURL("https://api.slack.test/"),
		slackapi.OptionHTTPClient(httpClient),
	)
	_, err := client.UploadImage(context.Background(), "C123", "", "snapshot.png", []byte("png"), "pane snapshot")
	if err == nil {
		t.Fatal("UploadImage() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "files:write") {
		t.Fatalf("UploadImage() error = %q, want files:write hint", err)
	}
}

func TestParseMultipartFileReadsUploadedBytes(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "snapshot.png")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write([]byte("png")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, "https://upload.slack.test/upload", io.NopCloser(bytes.NewReader(body.Bytes())))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	if got := string(parseMultipartFile(t, req)); got != "png" {
		t.Fatalf("parseMultipartFile() = %q, want png", got)
	}
}
