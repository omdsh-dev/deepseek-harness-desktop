//go:build !windows

// 孤儿后端清理：壳被强杀（SIGKILL/panic）时后端 dsh-server 无人接管，
// 残留为孤儿进程（其后 node 子进程一并残留）。每次启动时读取
// $DSH_HOME/shell.pid（上次会话记录的壳 PID 与后端进程组 PID）：若壳 PID
// 已死而后端进程组仍在，说明上次会话异常退出，按组终止残留后端并删除
// 记录。Windows 无需此机制：后端在 Job Object 中且 KILL_ON_JOB_CLOSE，
// 壳进程退出（含强杀）时句柄随进程关闭，作业树自动整组终止。
package supervise

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// pidFileName 是会话记录文件名（置于 DSH_HOME 根目录）。
const pidFileName = "shell.pid"

// pidFileGrace 是清扫时 SIGTERM 后等待进程组退出的宽限期，超时 SIGKILL 兜底。
const pidFileGrace = 3 * time.Second

// pidFilePath 返回会话记录文件路径。
func pidFilePath(dshHome string) string {
	return filepath.Join(dshHome, pidFileName)
}

// writePidFile 记录当前壳 PID 与后端进程组 PID（原子写：tmp + rename）。
func writePidFile(dshHome string, backendPID int) error {
	if backendPID <= 0 {
		return nil
	}
	path := pidFilePath(dshHome)
	raw := []byte(strconv.Itoa(os.Getpid()) + "\n" + strconv.Itoa(backendPID) + "\n")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// removePidFile 删除会话记录；仅当记录属于当前壳（避免误删并发实例记录）。
func removePidFile(dshHome string) {
	shellPID, _, ok := readPidFile(dshHome)
	if !ok || shellPID != os.Getpid() {
		return
	}
	_ = os.Remove(pidFilePath(dshHome))
}

// sweep 清扫上次异常退出的残留后端：壳 PID 已死且后端进程组仍在时按组终止。
func sweep(dshHome string) {
	shellPID, backendPID, ok := readPidFile(dshHome)
	if !ok {
		return
	}
	if alive(shellPID) {
		return // 上次会话仍在运行（并发实例），不动它的后端
	}
	if !groupAlive(backendPID) {
		_ = os.Remove(pidFilePath(dshHome))
		return // 后端已不在，仅清理残留记录
	}
	log.Printf("检测到上次会话异常退出（壳 %d 已退出），清理残留后端进程组 %d", shellPID, backendPID)
	killGroup(backendPID)
	_ = os.Remove(pidFilePath(dshHome))
}

// readPidFile 解析会话记录：两行，分别为壳 PID 与后端进程组 PID。
func readPidFile(dshHome string) (shellPID, backendPID int, ok bool) {
	raw, err := os.ReadFile(pidFilePath(dshHome))
	if err != nil {
		return 0, 0, false
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) < 2 {
		return 0, 0, false
	}
	a, err1 := strconv.Atoi(strings.TrimSpace(lines[0]))
	b, err2 := strconv.Atoi(strings.TrimSpace(lines[1]))
	if err1 != nil || err2 != nil || a <= 0 || b <= 0 {
		return 0, 0, false
	}
	return a, b, true
}

// alive 报告进程是否存在（signal 0 探测；EPERM 表示存在但无权发信号）。
func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// groupAlive 报告后端进程组是否仍在：进程存在且仍是其组组长（组 ID ==
// PID，即当初 Setpgid 建立的进程组未被解散/复用）。
func groupAlive(pid int) bool {
	pgid, err := syscall.Getpgid(pid)
	return err == nil && pgid == pid
}

// killGroup 终止进程组：先 SIGTERM 让其走收口路径（落盘会话等），宽限期
// 内未退出再 SIGKILL 兜底。
func killGroup(pgid int) {
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	deadline := time.Now().Add(pidFileGrace)
	for time.Now().Before(deadline) {
		err := syscall.Kill(-pgid, 0)
		if err == syscall.ESRCH {
			return // 进程组已解散
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
}
