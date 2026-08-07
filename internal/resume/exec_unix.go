//go:build !windows

package resume

import "syscall"

// ignoreSignal 是子进程退出时不需要转发的信号。
// Unix 上 SIGCHLD 是子进程退出通知，转发无意义且会干扰。
var ignoreSignal = syscall.SIGCHLD
