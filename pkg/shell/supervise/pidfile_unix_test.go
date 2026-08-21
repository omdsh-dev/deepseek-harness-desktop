//go:build !windows

package supervise

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// spawnOrphan 生成一个独立进程组里的长驻子进程（模拟残留后端 dsh-server）。
func spawnOrphan(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("spawn orphan: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		// 直接杀组长（沙箱环境可用）+ 组杀兜底（真实环境清整组）。
		_ = syscall.Kill(pid, syscall.SIGKILL)
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	})
	return pid
}

// canKillGroups 探测组信号是否真的能投递：探测进程若在组 SIGTERM 后退出，
// 说明 kill(-pgid) 生效。沙箱（seatbelt）可能静默丢弃组信号（syscall 返回
// 成功但不投递），此时无法在测试里验证进程终结本身。
func canKillGroups(t *testing.T) bool {
	t.Helper()
	probe := spawnOrphan(t)
	_ = syscall.Kill(-probe, syscall.SIGTERM)
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(probe, 0); err == syscall.ESRCH {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// waitGone 轮询等待进程组解散（最多 5s）。
func waitGone(t *testing.T, pgid int) bool {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(-pgid, 0); err == syscall.ESRCH {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func TestPidFileRoundtrip(t *testing.T) {
	home := t.TempDir()
	if err := writePidFile(home, 4242); err != nil {
		t.Fatalf("write: %v", err)
	}
	shell, backend, ok := readPidFile(home)
	if !ok || shell != os.Getpid() || backend != 4242 {
		t.Fatalf("roundtrip = %d/%d/%v, want %d/4242/true", shell, backend, ok, os.Getpid())
	}
}

func TestRemovePidFileOwnership(t *testing.T) {
	home := t.TempDir()

	// 自己的记录：可删除
	if err := writePidFile(home, 4242); err != nil {
		t.Fatalf("write: %v", err)
	}
	removePidFile(home)
	if _, err := os.Stat(pidFilePath(home)); !os.IsNotExist(err) {
		t.Fatal("自己的记录应被删除")
	}

	// 他人的记录（壳 PID 已死）：不应被删除
	os.WriteFile(pidFilePath(home), []byte("999999999\n4242\n"), 0o644)
	removePidFile(home)
	if _, err := os.Stat(pidFilePath(home)); err != nil {
		t.Fatal("他人的记录不应被删除")
	}
}

func TestSweepKillsOrphan(t *testing.T) {
	if alive(999999999) {
		t.Skip("PID 999999999 意外存活，跳过")
	}
	home := t.TempDir()
	orphan := spawnOrphan(t)
	// 记录：壳 PID 已死 + 孤儿后端进程组
	os.WriteFile(pidFilePath(home), []byte("999999999\n"+strconv.Itoa(orphan)+"\n"), 0o644)

	sweep(home)

	// 记录清理是 sweep 的决策结果，与信号可用性无关，始终断言。
	if _, err := os.Stat(pidFilePath(home)); !os.IsNotExist(err) {
		t.Fatal("sweep 后记录文件应被删除")
	}

	if canKillGroups(t) {
		if !waitGone(t, orphan) {
			t.Fatalf("孤儿进程组 %d 未被清理", orphan)
		}
	} else {
		// 沙箱静默丢弃组信号时无法验证进程终结；组杀与
		// server.Process.Stop 同语义（kill(-pgid)），真实环境可用。
		t.Logf("环境禁止组信号投递，跳过进程终结断言")
	}
}

func TestSweepKeepsLiveSession(t *testing.T) {
	home := t.TempDir()
	child := spawnOrphan(t)
	// 记录：当前壳（存活）+ 后端
	os.WriteFile(pidFilePath(home), []byte(strconv.Itoa(os.Getpid())+"\n"+strconv.Itoa(child)+"\n"), 0o644)

	sweep(home)

	if err := syscall.Kill(child, 0); err != nil {
		t.Fatalf("活壳会话的后端不应被清理: %v", err)
	}
	if _, err := os.Stat(pidFilePath(home)); err != nil {
		t.Fatal("活壳会话的记录文件不应被删除")
	}
}
