package main

import (
	"context"
	"log"
	"time"

	"github.com/omdsh-dev/deepseek-harness-desktop/server"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// 重启退避的初值与上限。
const (
	restartBackoff = time.Second
	maxRestartWait = 30 * time.Second
)

// supervise 守护后端：启动 → 就绪后把窗口指向其 URL → 进程退出则退避重启，
// 直到 ctx 取消（应用退出）。后端在任意时刻意外终结都会走同一重启路径。
func supervise(ctx context.Context, exeDir, profile, port, dshHome string, win *application.WebviewWindow) {
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

			// 等待本次进程终结（或应用退出）。
			select {
			case <-ctx.Done():
				// 应用退出：终止后端进程组并等待收口，不留孤儿 node。
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

		// 退避等待后重启；应用退出则立即结束。
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
