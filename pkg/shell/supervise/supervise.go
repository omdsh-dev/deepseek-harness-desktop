// Package supervise 守护 SEA 后端（dsh-server）进程：启动、把窗口指向其
// URL、异常退出退避重启，直到上下文取消（应用退出）。
package supervise

import (
	"context"
	"log"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/omdsh-dev/dsh-web-desktopify/pkg/shell/server"
)

// 重启退避的初值与上限。
const (
	restartBackoff = time.Second
	maxRestartWait = 30 * time.Second
)

// Run 守护后端：启动 → 就绪后把窗口指向其 URL → 进程退出则退避重启，
// 直到 ctx 取消（应用退出）。
func Run(ctx context.Context, exeDir, profile, port, dshHome string, win *application.WebviewWindow) {
	backoff := restartBackoff
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		p, url, err := server.Start(ctx, exeDir, profile, port, dshHome)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("dsh server 启动失败：%v（%s 后重试）", err, backoff)
		} else {
			backoff = restartBackoff
			log.Printf("dsh server ready at %s", url)
			win.SetURL(url)

			select {
			case <-ctx.Done():
				p.Stop()
				return
			case exit := <-p.Exit():
				if exit.Err != nil {
					log.Printf("dsh server 异常退出：%v", exit.Err)
				} else {
					log.Printf("dsh server 退出（重启）")
				}
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < maxRestartWait {
			backoff *= 2
			if backoff > maxRestartWait {
				backoff = maxRestartWait
			}
		}
	}
}
