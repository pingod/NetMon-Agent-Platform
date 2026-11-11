package ops

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// localDockerClient 通过本机 docker CLI + /var/run/docker.sock 执行命令。
// 与 SSH 客户端返回同样的 stdout/stderr/err，复用现有解析逻辑。
type localDockerClient struct{}

func ensureLocalDocker() error {
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		return fmt.Errorf("本机未挂载 /var/run/docker.sock")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("已挂载 docker.sock 但镜像内缺少 docker CLI")
	}
	return nil
}

func (l *localDockerClient) Close() error { return nil }

func (l *localDockerClient) ExecCommand(command string, timeout time.Duration) (string, string, error) {
	return l.ExecCommandInput(command, "", timeout)
}

func (l *localDockerClient) ExecCommandInput(command string, input string, timeout time.Duration) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return stdout.String(), stderr.String(), fmt.Errorf("command timed out after %v", timeout)
	}
	return stdout.String(), stderr.String(), err
}
