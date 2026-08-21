//go:build windows

package supervise

// Windows 上后端在 Job Object 中且 KILL_ON_JOB_CLOSE：壳进程退出（含强杀、
// 崩溃）时句柄随进程关闭，作业树自动整体终止，无需孤儿清扫。这里提供
// 空实现，保持 Run 的调用点跨平台一致。

func writePidFile(_ string, _ int) error { return nil }

func removePidFile(string) {}

func sweep(string) {}
