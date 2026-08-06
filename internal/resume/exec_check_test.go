package resume

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/talea/talea/internal/adapters"
)

// TestExecReplacesProcess 验证 syscall.Exec 替换进程。
// 通过 -run=TestHelperExec 子进程方式：主进程用 echo 替换后输出标记。
func TestExecReplacesProcess(t *testing.T) {
	if os.Getenv("BE_EXEC_HELPER") == "1" {
		plan := Plan{
			TargetDir: t.TempDir(),
			Command:   adapters.Command{Program: "echo", Args: []string{"RESUME-EXEC-OK"}},
		}
		if err := Exec(plan); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestExecReplacesProcess")
	cmd.Env = append(os.Environ(), "BE_EXEC_HELPER=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helper failed: %v out=%s", err, out)
	}
	// resetTerminal 会在 Exec 前输出终端恢复序列，标记应出现在输出中
	if !strings.Contains(string(out), "RESUME-EXEC-OK") {
		t.Fatalf("unexpected output: %q", string(out))
	}
}
