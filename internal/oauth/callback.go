package oauth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// CallbackServer 是本地 OAuth 回调服务器。
// 它启动 HTTP 服务器监听本地端口，等待授权服务器将授权码回调到本地。
type CallbackServer struct {
	server   *http.Server
	codeChan chan string
	errChan  chan error
	started  bool
}

// NewCallbackServer 创建新的回调服务器实例。
func NewCallbackServer() *CallbackServer {
	return &CallbackServer{}
}

// Start 启动本地回调服务器，返回完整的回调 URL（包含路径）。
// 如果 port 为 0，由操作系统分配可用端口。
func (cs *CallbackServer) Start(port int) (string, error) {
	if cs.started {
		return "", fmt.Errorf("回调服务器已启动")
	}

	cs.codeChan = make(chan string, 1)
	cs.errChan = make(chan error, 1)

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return "", fmt.Errorf("监听端口 %d 失败: %w", port, err)
	}

	// 如果 port=0，获取实际分配的端口
	actualPort := listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", cs.handleCallback)

	cs.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	cs.started = true

	// 后台启动 HTTP 服务
	go func() {
		if err := cs.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			cs.errChan <- fmt.Errorf("回调服务器错误: %w", err)
		}
	}()

	return fmt.Sprintf("http://127.0.0.1:%d/callback", actualPort), nil
}

// handleCallback 处理 OAuth 授权服务器的回调请求。
func (cs *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// 检查是否有错误回调
	if errStr := query.Get("error"); errStr != "" {
		errDesc := query.Get("error_description")
		err := fmt.Errorf("OAuth 授权错误: %s", errStr)
		if errDesc != "" {
			err = fmt.Errorf("OAuth 授权错误: %s (%s)", errDesc, errStr)
		}

		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, "授权失败: %v\n", err)
		cs.errChan <- err
		return
	}

	// 提取授权码
	code := query.Get("code")
	if code == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, "缺少授权码参数\n")
		cs.errChan <- fmt.Errorf("回调中缺少授权码参数")
		return
	}

	_, _ = fmt.Fprint(w, "授权成功，可以关闭此窗口。\n")
	cs.codeChan <- code
}

// WaitForCode 等待授权码到达。
// 支持通过 context 取消等待。
func (cs *CallbackServer) WaitForCode(ctx context.Context) (string, error) {
	if !cs.started {
		return "", fmt.Errorf("回调服务器未启动")
	}

	select {
	case code := <-cs.codeChan:
		return code, nil
	case err := <-cs.errChan:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Stop 停止回调服务器。
func (cs *CallbackServer) Stop() error {
	if !cs.started {
		return nil
	}
	cs.started = false
	return cs.server.Close()
}