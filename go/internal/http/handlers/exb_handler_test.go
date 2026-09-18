package handlers

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cadguanliq/internal/auth"
	"cadguanliq/internal/http/middleware"
)

// multipartUploadRequest 构造一个已登录的 multipart 上传请求（文件内容不重要，识别只看文件名）。
func multipartUploadRequest(t *testing.T, filename string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("EXB")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/exb/identify", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request.WithContext(context.WithValue(request.Context(), middleware.AuthUserContextKey, auth.AuthUser{ID: "test-user"}))
}

func TestResolvePartNo(t *testing.T) {
	tests := []struct {
		name       string
		filename   string
		titleBlock map[string]string
		want       string
		source     string
	}{
		{name: "文件名优先避免标题栏误读", filename: "A-001.dwg", titleBlock: map[string]string{"图号": "B-002"}, want: "A-001", source: "filename"},
		{name: "标题栏缺失回退文件名", filename: "A-001-02（支架）.dwg", titleBlock: map[string]string{}, want: "A-001-02", source: "filename"},
		{name: "无法识别返回空", filename: "支架.dwg", titleBlock: map[string]string{}, want: "", source: "none"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, source := resolvePartNo(test.filename, test.titleBlock, false)
			if got != test.want || source != test.source {
				t.Fatalf("resolvePartNo() = %q, %q; want %q, %q", got, source, test.want, test.source)
			}
		})
	}
}

func TestResolvePartNoRejectsTitleBlockNoise(t *testing.T) {
	filename := "JG9055e-5032-01(缸体).exb"
	for _, value := range []string{"4-MN-12WD", "0.02", "GB1235-76"} {
		got, source := resolvePartNo(filename, map[string]string{"图号": value}, false)
		if got != "JG9055e-5032-01" || source != "filename" {
			t.Fatalf("resolvePartNo(%q) = %q, %q; want filename fallback", value, got, source)
		}
	}
}

func TestResolvePartNoTitleBlockOnly(t *testing.T) {
	got, source := resolvePartNo("JG9055e-5032-00(总图).exb", map[string]string{"图号": "2000W.02.03E-01-3"}, true)
	if got != "2000W.02.03E-01-3" || source != "titleBlock" {
		t.Fatalf("resolvePartNo() = %q, %q; want title block value", got, source)
	}
	got, source = resolvePartNo("JG9055e-5032-00(总图).exb", map[string]string{"图号": "0.02"}, true)
	if got != "" || source != "none" {
		t.Fatalf("resolvePartNo() = %q, %q; want empty result for noise", got, source)
	}
}

func TestResolvePartNoNormalizesTitleBlockKeyWhitespace(t *testing.T) {
	got, source := resolvePartNo("fallback-01(零件).exb", map[string]string{"图纸 编号": "JG1285-250/180-3255%x4070"}, true)
	if got != "JG1285-250/180-3255%x4070" || source != "titleBlock" {
		t.Fatalf("resolvePartNo() = %q, %q; want normalized title block value", got, source)
	}
}

func TestFallbackPartNoAllowsPercentDimensions(t *testing.T) {
	got, source := resolvePartNo("JG1285-250-180-3255%x4070(液压缸).exb", nil, false)
	if got != "JG1285-250-180-3255%x4070" || source != "filename" {
		t.Fatalf("resolvePartNo() = %q, %q; want complete filename fallback", got, source)
	}
}

func TestIsLikelyDrawingNoRejectsDimensions(t *testing.T) {
	for _, value := range []string{"4-MN-12WD", "0.02", "GB1235-76", "1.2:1"} {
		if isLikelyDrawingNo(value) {
			t.Fatalf("isLikelyDrawingNo(%q) = true", value)
		}
	}
	for _, value := range []string{"2000W.02.03E-01-3", "JG9055e-5032-01", "JG9055e-50/32-00", "JG1285-250/180-3255%x4070"} {
		if !isLikelyDrawingNo(value) {
			t.Fatalf("isLikelyDrawingNo(%q) = false", value)
		}
	}
}

// 前端已改为读图幅；本接口是兜底。文件名里没有图号时必须回 422 并说明怎么改，
// 而不是回 200 + 空图号（那会让前端抛「未返回有效内部图号」，用户看不出该怎么办）。
func TestIdentifyDrawingFileRejectsFilenameWithoutPartNo(t *testing.T) {
	request := multipartUploadRequest(t, "工程图文档2.exb")
	recorder := httptest.NewRecorder()
	IdentifyDrawingFile(nil)(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body=%s; want 422", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "文件名") {
		t.Fatalf("body = %s; want actionable filename hint", recorder.Body.String())
	}
}

func TestIdentifyDrawingFileAcceptsFilenamePartNo(t *testing.T) {
	request := multipartUploadRequest(t, "JG9055e-5032-01(缸体).exb")
	recorder := httptest.NewRecorder()
	IdentifyDrawingFile(nil)(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s; want 200", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "JG9055e-5032-01") {
		t.Fatalf("body = %s; want resolved part number", recorder.Body.String())
	}
}
