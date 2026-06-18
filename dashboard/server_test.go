package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkit/cron"
	"github.com/darkit/cron/history"
)

func newDashboardHTTPServer(t *testing.T, server *Server) *httptest.Server {
	t.Helper()
	handler, err := server.buildHandler()
	if err != nil {
		t.Fatalf("failed to build handler: %v", err)
	}
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

func decodeSuccessResponse(t *testing.T, resp *http.Response) SuccessResponse {
	t.Helper()
	defer resp.Body.Close()
	var payload SuccessResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode success response: %v", err)
	}
	return payload
}

func decodeErrorResponse(t *testing.T, resp *http.Response) ErrorResponse {
	t.Helper()
	defer resp.Body.Close()
	var payload ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	return payload
}

// TestNewServer 测试创建服务器
func TestNewServer(t *testing.T) {
	c := cron.New()
	defer c.Stop()

	server := NewServer(c, ":9999")

	if server == nil {
		t.Fatal("Expected non-nil server")
	}

	if server.addr != ":9999" {
		t.Errorf("Expected addr ':9999', got '%s'", server.addr)
	}

	if server.cron != c {
		t.Error("Server cron instance mismatch")
	}
}

// TestNewServerDefaultAddr 测试默认地址
func TestNewServerDefaultAddr(t *testing.T) {
	c := cron.New()
	defer c.Stop()

	server := NewServer(c, "")

	if server.addr != ":8080" {
		t.Errorf("Expected default addr ':8080', got '%s'", server.addr)
	}
}

// TestServerStartStop 测试服务器启动和停止
func TestServerStartStop(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建历史记录存储
	storage, err := history.NewFileStorage(filepath.Join(tmpDir, "history"))
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	recorder, err := history.NewHistoryRecorder(storage)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer recorder.Close()

	c := cron.New(cron.WithHistoryRecorder(recorder))

	// 添加测试任务
	c.Schedule("server-test-task", "@every 2s", func(ctx context.Context) {
		time.Sleep(10 * time.Millisecond)
	})

	c.Start()
	defer c.Stop()

	// 使用一个随机端口避免冲突
	server := NewServer(c, "127.0.0.1:0")

	// 启动服务器
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)
	baseURL := "http://" + server.addr

	// 测试 API 端点是否可访问
	resp, err := http.Get(baseURL + "/api/tasks")
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	var tasks []TaskInfo
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(tasks) == 0 {
		t.Error("Expected at least one task")
	}

	// 停止服务器
	if err := server.Stop(); err != nil {
		t.Errorf("Failed to stop server: %v", err)
	}

	// 验证服务器已停止（应该无法连接）
	time.Sleep(100 * time.Millisecond)
	_, err = http.Get(baseURL + "/api/tasks")
	if err == nil {
		t.Error("Server should have been stopped")
	}
}

// TestServerAPIEndpoints 测试所有 API 端点
func TestServerAPIEndpoints(t *testing.T) {
	tmpDir := t.TempDir()

	storage, err := history.NewFileStorage(filepath.Join(tmpDir, "history"))
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	recorder, err := history.NewHistoryRecorder(storage)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer recorder.Close()

	c := cron.New(cron.WithHistoryRecorder(recorder))

	c.Schedule("endpoint-test-task", "@every 1s", func(ctx context.Context) {
		time.Sleep(10 * time.Millisecond)
	})

	c.Start()
	defer c.Stop()

	server := NewServer(c, "127.0.0.1:0")
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	time.Sleep(100 * time.Millisecond)
	baseURL := "http://" + server.addr

	// 测试各个端点
	endpoints := []struct {
		path       string
		statusCode int
	}{
		{"/api/tasks", http.StatusOK},
		{"/api/stats", http.StatusOK},
		{"/api/history", http.StatusOK},
		{"/api/tasks/endpoint-test-task", http.StatusOK},
		{"/api/tasks/non-existent", http.StatusNotFound},
	}

	for _, endpoint := range endpoints {
		resp, err := http.Get(baseURL + endpoint.path)
		if err != nil {
			t.Errorf("Failed to access %s: %v", endpoint.path, err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode != endpoint.statusCode {
			t.Errorf("Endpoint %s: expected status %d, got %d",
				endpoint.path, endpoint.statusCode, resp.StatusCode)
		}
	}
}

// TestServerCORS 测试 CORS 中间件
func TestServerCORS(t *testing.T) {
	c := cron.New()
	defer c.Stop()

	server := NewServer(c, "127.0.0.1:0")
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	time.Sleep(100 * time.Millisecond)
	baseURL := "http://" + server.addr

	// 创建带有 Origin 头的请求
	req, err := http.NewRequest("GET", baseURL+"/api/tasks", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Origin", "http://example.com")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// 验证 CORS 头
	corsOrigin := resp.Header.Get("Access-Control-Allow-Origin")
	if corsOrigin != "http://example.com" {
		t.Errorf("Expected reflected CORS origin, got '%s'", corsOrigin)
	}

	corsMethods := resp.Header.Get("Access-Control-Allow-Methods")
	if corsMethods == "" {
		t.Error("CORS methods header not set")
	}
}

func TestServerCORSAllowsNoOriginWhenRestricted(t *testing.T) {
	c := cron.New()
	defer c.Stop()

	server := NewServer(c, ":0", WithAllowedOrigins([]string{"http://example.com"}))
	handler := server.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected request without Origin to pass through, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no CORS origin header for requests without Origin, got %q", got)
	}
}

func TestServerCORSRejectsDisallowedOriginAsJSON(t *testing.T) {
	c := cron.New()
	defer c.Stop()

	server := NewServer(c, ":0", WithAllowedOrigins([]string{"https://allowed.example.com"}))
	handler := server.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	req.Header.Set("Origin", "https://blocked.example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for blocked origin, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("expected json content type, got %q", got)
	}
	var payload ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if payload.Message != "Origin not allowed" {
		t.Fatalf("unexpected error message: %q", payload.Message)
	}
	if vary := rec.Header().Values("Vary"); len(vary) == 0 || vary[0] != "Origin" {
		t.Fatalf("expected Vary: Origin, got %v", vary)
	}
}

func TestServerCORSPreflightSetsVaryHeaders(t *testing.T) {
	c := cron.New()
	defer c.Stop()

	server := NewServer(c, ":0", WithAllowedOrigins([]string{"https://allowed.example.com"}))
	handler := server.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/tasks", nil)
	req.Header.Set("Origin", "https://allowed.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPatch)
	req.Header.Set("Access-Control-Request-Headers", "Authorization, X-API-Key")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for preflight, got %d", rec.Code)
	}
	vary := rec.Header().Values("Vary")
	joined := strings.Join(vary, ",")
	for _, expected := range []string{"Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected Vary to contain %q, got %v", expected, vary)
		}
	}
}

func TestServerUnfuseCompatibilityAlias(t *testing.T) {
	c := cron.New()
	defer c.Stop()

	if err := c.Schedule("alias-task", "@every 1m", func(ctx context.Context) {}); err != nil {
		t.Fatalf("schedule failed: %v", err)
	}

	server := NewServer(c, ":0")
	handler := server.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.handler.UnfuseTask(w, r)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/tasks/alias-task/unfuse", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected unfuse alias to succeed, got %d body=%s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "compatibility alias") {
		t.Fatalf("expected compatibility alias message, got %s", body)
	}
}

func TestServerWriteEndpointsLifecycle(t *testing.T) {
	c := cron.New()
	defer c.Stop()

	runCh := make(chan struct{}, 1)
	if err := c.Schedule("lifecycle-task", "@every 24h", func(ctx context.Context) {
		select {
		case runCh <- struct{}{}:
		default:
		}
	}); err != nil {
		t.Fatalf("schedule failed: %v", err)
	}
	if err := c.Start(); err != nil {
		t.Fatalf("start scheduler failed: %v", err)
	}
	defer c.Stop()

	server := NewServer(c, ":0")
	ts := newDashboardHTTPServer(t, server)
	client := ts.Client()

	request := func(method, path string, body io.Reader) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, ts.URL+path, body)
		if err != nil {
			t.Fatalf("create request failed: %v", err)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request %s %s failed: %v", method, path, err)
		}
		return resp
	}

	resp := request(http.MethodPost, "/api/tasks/lifecycle-task/pause", nil)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("pause failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	if msg := decodeSuccessResponse(t, resp).Message; msg != "Task paused" {
		t.Fatalf("unexpected pause message: %q", msg)
	}
	if task, ok := c.GetTask("lifecycle-task"); !ok || !task.IsPaused {
		t.Fatalf("expected task paused, task=%v ok=%v", task, ok)
	}

	resp = request(http.MethodPost, "/api/tasks/lifecycle-task/resume", nil)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("resume failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	if msg := decodeSuccessResponse(t, resp).Message; msg != "Task resumed" {
		t.Fatalf("unexpected resume message: %q", msg)
	}
	if task, ok := c.GetTask("lifecycle-task"); !ok || task.IsPaused {
		t.Fatalf("expected task resumed, task=%v ok=%v", task, ok)
	}

	resp = request(http.MethodPost, "/api/tasks/lifecycle-task/run", nil)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("run failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	if msg := decodeSuccessResponse(t, resp).Message; msg != "Task triggered" {
		t.Fatalf("unexpected run message: %q", msg)
	}
	select {
	case <-runCh:
	case <-time.After(2 * time.Second):
		t.Fatal("expected task to execute after /run")
	}

	resp = request(http.MethodPatch, "/api/tasks/lifecycle-task/schedule", bytes.NewBufferString(`{"schedule":"@every 5m"}`))
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("schedule update failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	if msg := decodeSuccessResponse(t, resp).Message; msg != "Schedule updated" {
		t.Fatalf("unexpected update message: %q", msg)
	}

	resp, err := client.Get(ts.URL + "/api/tasks/lifecycle-task")
	if err != nil {
		t.Fatalf("get task failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get task after update failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	var task TaskInfo
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		resp.Body.Close()
		t.Fatalf("decode task failed: %v", err)
	}
	resp.Body.Close()
	if task.Schedule != "@every 5m" {
		t.Fatalf("expected updated schedule, got %q", task.Schedule)
	}

	resp = request(http.MethodDelete, "/api/tasks/lifecycle-task", nil)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("delete failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	if msg := decodeSuccessResponse(t, resp).Message; msg != "Task removed successfully" {
		t.Fatalf("unexpected delete message: %q", msg)
	}

	resp, err = client.Get(ts.URL + "/api/tasks/lifecycle-task")
	if err != nil {
		t.Fatalf("get deleted task failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected deleted task to return 404, got %d body=%s", resp.StatusCode, string(body))
	}
}

func TestServerWriteEndpointsRequireAPIKey(t *testing.T) {
	c := cron.New()
	defer c.Stop()

	runCh := make(chan struct{}, 1)
	if err := c.Schedule("auth-task", "@every 24h", func(ctx context.Context) {
		select {
		case runCh <- struct{}{}:
		default:
		}
	}); err != nil {
		t.Fatalf("schedule failed: %v", err)
	}
	if err := c.Start(); err != nil {
		t.Fatalf("start scheduler failed: %v", err)
	}
	defer c.Stop()

	server := NewServer(c, ":0", WithAPIKey("secret-key"))
	ts := newDashboardHTTPServer(t, server)
	client := ts.Client()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/tasks/auth-task/pause", nil)
	if err != nil {
		t.Fatalf("create unauthorized request failed: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do unauthorized request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 401 without api key, got %d body=%s", resp.StatusCode, string(body))
	}
	if payload := decodeErrorResponse(t, resp); payload.Message != "Invalid or missing API key" {
		t.Fatalf("unexpected unauthorized message: %q", payload.Message)
	}

	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/tasks/auth-task/pause", nil)
	req.Header.Set("X-API-Key", "secret-key")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("pause with header api key failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 200 with header api key, got %d body=%s", resp.StatusCode, string(body))
	}
	decodeSuccessResponse(t, resp)

	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/tasks/auth-task/resume?api_key=secret-key", nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("resume with query api key failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 200 with query api key, got %d body=%s", resp.StatusCode, string(body))
	}
	decodeSuccessResponse(t, resp)

	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/tasks/auth-task/run", nil)
	req.Header.Set("Authorization", "Bearer secret-key")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("run with bearer api key failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 200 with bearer api key, got %d body=%s", resp.StatusCode, string(body))
	}
	decodeSuccessResponse(t, resp)

	select {
	case <-runCh:
	case <-time.After(2 * time.Second):
		t.Fatal("expected authorized /run request to execute task")
	}
}

func TestServerWriteEndpointErrorContracts(t *testing.T) {
	c := cron.New()
	defer c.Stop()
	if err := c.Schedule("error-task", "@every 24h", func(ctx context.Context) {}); err != nil {
		t.Fatalf("schedule failed: %v", err)
	}

	server := NewServer(c, ":0")
	ts := newDashboardHTTPServer(t, server)
	client := ts.Client()

	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantMsg    string
	}{
		{name: "run missing task", method: http.MethodPost, path: "/api/tasks/missing/run", wantStatus: http.StatusBadRequest, wantMsg: "task missing not found"},
		{name: "pause missing task", method: http.MethodPost, path: "/api/tasks/missing/pause", wantStatus: http.StatusBadRequest, wantMsg: "task missing not found"},
		{name: "resume missing task", method: http.MethodPost, path: "/api/tasks/missing/resume", wantStatus: http.StatusBadRequest, wantMsg: "task missing not found"},
		{name: "schedule missing task", method: http.MethodPatch, path: "/api/tasks/missing/schedule", body: `{"schedule":"@every 5m"}`, wantStatus: http.StatusBadRequest, wantMsg: "task missing not found"},
		{name: "delete missing task", method: http.MethodDelete, path: "/api/tasks/missing", wantStatus: http.StatusNotFound, wantMsg: "task missing not found"},
		{name: "schedule invalid json", method: http.MethodPatch, path: "/api/tasks/error-task/schedule", body: `{"schedule":`, wantStatus: http.StatusBadRequest, wantMsg: "invalid request body"},
		{name: "schedule blank", method: http.MethodPatch, path: "/api/tasks/error-task/schedule", body: `{"schedule":"   "}`, wantStatus: http.StatusBadRequest, wantMsg: "schedule is required"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body io.Reader
			if tc.body != "" {
				body = bytes.NewBufferString(tc.body)
			}
			req, err := http.NewRequest(tc.method, ts.URL+tc.path, body)
			if err != nil {
				t.Fatalf("create request failed: %v", err)
			}
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			if resp.StatusCode != tc.wantStatus {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				t.Fatalf("expected status %d, got %d body=%s", tc.wantStatus, resp.StatusCode, string(body))
			}
			payload := decodeErrorResponse(t, resp)
			if !strings.Contains(payload.Message, tc.wantMsg) {
				t.Fatalf("expected error message containing %q, got %q", tc.wantMsg, payload.Message)
			}
		})
	}
}

func TestServerMethodMismatchReturns405(t *testing.T) {
	c := cron.New()
	defer c.Stop()
	if err := c.Schedule("method-task", "@every 24h", func(ctx context.Context) {}); err != nil {
		t.Fatalf("schedule failed: %v", err)
	}

	server := NewServer(c, ":0")
	ts := newDashboardHTTPServer(t, server)
	resp, err := ts.Client().Get(ts.URL + "/api/tasks/method-task/pause")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 405 for method mismatch, got %d body=%s", resp.StatusCode, string(body))
	}
}

func TestServerStartReturnsErrorOnPortConflict(t *testing.T) {
	c := cron.New()
	defer c.Stop()
	server1 := NewServer(c, "127.0.0.1:0")
	if err := server1.Start(); err != nil {
		t.Fatalf("first start failed: %v", err)
	}
	defer server1.Stop()
	server2 := NewServer(c, server1.server.Addr)
	if err := server2.Start(); err == nil {
		t.Fatal("expected port conflict error")
	}
}
