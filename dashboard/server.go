package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/darkit/cron"
)

//go:embed web/*
var webFS embed.FS

// ServerOption 服务器配置选项
type ServerOption func(*Server)

// WithAPIKey 设置 API Key 认证（空字符串表示禁用认证）
func WithAPIKey(apiKey string) ServerOption {
	return func(s *Server) {
		s.apiKey = apiKey
	}
}

// WithAllowedOrigins 设置允许的 CORS 来源（空切片表示允许所有来源）
func WithAllowedOrigins(origins []string) ServerOption {
	return func(s *Server) {
		s.allowedOrigins = origins
	}
}

// Server Dashboard 服务器
type Server struct {
	cron           *cron.Cron
	handler        *Handler
	server         *http.Server
	addr           string
	logger         *log.Logger
	apiKey         string   // API Key 认证（空表示禁用）
	allowedOrigins []string // 允许的 CORS 来源（空表示允许所有）
}

// WithLogger 设置 Dashboard 使用的标准库 logger。
// 传入 nil 时保持默认 logger。
func WithLogger(logger *log.Logger) ServerOption {
	return func(s *Server) {
		if logger != nil {
			s.logger = logger
		}
	}
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error:   http.StatusText(status),
		Message: message,
		Code:    status,
	})
}

// NewServer 创建新的 Dashboard 服务器
func NewServer(c *cron.Cron, addr string, opts ...ServerOption) *Server {
	if addr == "" {
		addr = ":8080"
	}

	s := &Server{
		cron:    c,
		handler: NewHandler(c),
		addr:    addr,
		logger:  log.Default(),
	}

	// 应用选项
	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (s *Server) buildHandler() (http.Handler, error) {
	rootMux := http.NewServeMux()
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /tasks", s.handler.GetTasks)
	apiMux.HandleFunc("GET /tasks/{id}", s.handler.GetTask)
	apiMux.HandleFunc("DELETE /tasks/{id}", s.handler.RemoveTask)
	apiMux.HandleFunc("POST /tasks/{id}/run", s.handler.RunTaskNow)
	apiMux.HandleFunc("POST /tasks/{id}/pause", s.handler.PauseTask)
	apiMux.HandleFunc("POST /tasks/{id}/resume", s.handler.ResumeTask)
	apiMux.HandleFunc("POST /tasks/{id}/unfuse", s.handler.UnfuseTask)
	apiMux.HandleFunc("PATCH /tasks/{id}/schedule", s.handler.UpdateTaskSchedule)
	apiMux.HandleFunc("GET /stats", s.handler.GetStats)
	apiMux.HandleFunc("GET /history", s.handler.GetHistory)

	apiHandler := http.StripPrefix("/api", apiMux)
	if s.apiKey != "" {
		apiHandler = s.authMiddleware(apiHandler)
	}
	rootMux.Handle("/api/", apiHandler)

	webRoot, err := fs.Sub(webFS, "web")
	if err != nil {
		return nil, fmt.Errorf("failed to get web root: %w", err)
	}
	webHandler := http.FileServer(http.FS(webRoot))
	if s.apiKey != "" {
		webHandler = s.authMiddleware(webHandler)
	}
	rootMux.Handle("/", webHandler)

	return s.corsMiddleware(rootMux), nil
}

// Start 启动 Dashboard 服务器
func (s *Server) Start() error {
	handler, err := s.buildHandler()
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.addr = ln.Addr().String()

	s.server = &http.Server{
		Addr:         s.addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if s.logger != nil {
		s.logger.Printf("Dashboard server starting on %s", s.addr)
		s.logger.Println("API endpoints:")
		s.logger.Println("  GET    /api/tasks           - 获取所有任务")
		s.logger.Println("  GET    /api/tasks/{id}      - 获取任务详情")
		s.logger.Println("  DELETE /api/tasks/{id}      - 移除任务")
		s.logger.Println("  POST   /api/tasks/{id}/run  - 立即触发任务")
		s.logger.Println("  POST   /api/tasks/{id}/pause - 暂停任务")
		s.logger.Println("  POST   /api/tasks/{id}/resume - 恢复任务（canonical）")
		s.logger.Println("  POST   /api/tasks/{id}/unfuse - 兼容旧版解除熔断别名")
		s.logger.Println("  PATCH  /api/tasks/{id}/schedule - 更新调度表达式")
		s.logger.Println("  GET    /api/stats           - 获取统计信息")
		s.logger.Println("  GET    /api/history         - 获取历史记录")
		s.logger.Println("")
		s.logger.Println("Query parameters for /api/history:")
		s.logger.Println("  taskId      - 按任务ID筛选")
		s.logger.Println("  successOnly - 仅成功记录 (true/false)")
		s.logger.Println("  failedOnly  - 仅失败记录 (true/false)")
		s.logger.Println("  startTime   - 开始时间 (RFC3339格式)")
		s.logger.Println("  endTime     - 结束时间 (RFC3339格式)")
		s.logger.Println("  limit       - 每页记录数 (默认50)")
		s.logger.Println("  offset      - 偏移量 (默认0)")
	}

	// 在新 goroutine 中启动服务器
	go func() {
		if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			if s.logger != nil {
				s.logger.Printf("Dashboard server error: %v", err)
			}
		}
	}()

	return nil
}

// Stop 停止 Dashboard 服务器
func (s *Server) Stop() error {
	if s.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if s.logger != nil {
		s.logger.Println("Stopping dashboard server...")
	}
	return s.server.Shutdown(ctx)
}

// corsMiddleware CORS 中间件
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Add("Vary", "Origin")
		}

		// 确定允许的来源
		allowedOrigin := ""
		if len(s.allowedOrigins) == 0 {
			if origin != "" {
				allowedOrigin = origin
			}
		} else {
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			allowedOrigin = ""
			for _, allowed := range s.allowedOrigins {
				if allowed == origin || allowed == "*" {
					allowedOrigin = origin
					break
				}
			}
			if allowedOrigin == "" {
				// 来源不在允许列表中
				writeJSONError(w, http.StatusForbidden, "Origin not allowed")
				return
			}
		}

		if allowedOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")

		if r.Method == "OPTIONS" {
			if r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Add("Vary", "Access-Control-Request-Method")
			}
			if r.Header.Get("Access-Control-Request-Headers") != "" {
				w.Header().Add("Vary", "Access-Control-Request-Headers")
			}
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// authMiddleware API Key 认证中间件
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 从 Header 或 Query 参数获取 API Key
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			apiKey = r.URL.Query().Get("api_key")
		}
		if apiKey == "" {
			// 尝试从 Authorization Bearer 获取
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				apiKey = strings.TrimPrefix(auth, "Bearer ")
			}
		}

		if apiKey != s.apiKey {
			writeJSONError(w, http.StatusUnauthorized, "Invalid or missing API key")
			return
		}

		next.ServeHTTP(w, r)
	})
}
