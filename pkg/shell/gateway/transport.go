package gateway

import (
	"context"
	"net/http"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Transport 是 wails 自定义 IPC transport：
//   - Start 时拿到 MessageProcessor，网关的 /wails/runtime（IPC）转发给它
//     完成绑定分发——外部 URL 页面（dsh web）无需加载 wails assetserver
//     即可用 window.wails.Call.ByName。
//   - ServeAssets 时拿到 wails assetserver handler（含 /wails/runtime.js
//     伺服），网关的 /wails/runtime.js 路由转发给它——runtime.js 由 wails
//     提供，壳无需自己 embed。
type Transport struct {
	gw *Gateway
}

// NewTransport 创建绑定到网关的 transport。
func NewTransport(gw *Gateway) *Transport {
	return &Transport{gw: gw}
}

// Start 把 MessageProcessor 注入网关（wails 在应用初始化时调用）。
func (t *Transport) Start(_ context.Context, mp *application.MessageProcessor) error {
	t.gw.SetMessageProcessor(mp)
	return nil
}

// ServeAssets 把 wails assetserver handler 交给网关（伺服 runtime.js 等）。
func (t *Transport) ServeAssets(assetHandler http.Handler) error {
	t.gw.SetAssetHandler(assetHandler)
	return nil
}

// JSClient 无独立 JS 客户端（runtime.js 由 wails assetserver 提供）。
func (t *Transport) JSClient() []byte { return nil }

// Stop 无资源需要释放。
func (t *Transport) Stop() error { return nil }
