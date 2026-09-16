package health

import (
	"encoding/json"
	"net/http"

	"zhongkui.com/zhongkui/internal/pkg/status"
)

// probeOK 写入 200 响应，正文为给定文本。
func probeOK(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

// probeBody 写入 503 响应，正文为给定文本。
func probeBody(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(body))
}

// probeBodyDeps 写入 503 响应，正文为依赖状态列表的 JSON。
func probeBodyDeps(w http.ResponseWriter, deps []status.Status) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(deps)
}

// handleLive 存活探针：监听中且未进入关机即为存活。
func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	live := s.listening.Load() && !s.shuttingDown.Load()
	s.metrics.record(r.Context(), live, false, s.started.Load(), nil)
	if live {
		probeOK(w, "ok")
		return
	}
	probeBody(w, "shutting down")
}

// handleStartup 启动探针：完成启动（ReportStarted）即为就绪。
func (s *Server) handleStartup(w http.ResponseWriter, r *http.Request) {
	started := s.started.Load()
	s.metrics.record(r.Context(), s.listening.Load() && !s.shuttingDown.Load(), false, started, nil)
	if started {
		probeOK(w, "ok")
		return
	}
	probeBody(w, "starting up")
}

// handleReady 就绪探针：已启动、未关机且全部已配置依赖可达时才返回 200；
// 否则返回 503 并携带 JSON 化的依赖状态列表说明不就绪原因。
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	started := s.started.Load()
	shutting := s.shuttingDown.Load()
	live := s.listening.Load() && !shutting

	ready := started && !shutting
	var results []status.Status
	if ready {
		results = s.reg.Run(r.Context())
		ready = allStateReady(results)
	}
	s.metrics.record(r.Context(), live, ready, started, results)

	if ready {
		probeOK(w, "ok")
		return
	}
	probeBodyDeps(w, results)
}