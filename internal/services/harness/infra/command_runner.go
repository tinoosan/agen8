package infra

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
)

type LocalCommandRunner struct{}

func (LocalCommandRunner) StartCommand(ctx context.Context, spec domain.StartSpec) (domain.CommandProcess, error) {
	if strings.TrimSpace(spec.Command) == "" {
		return nil, fmt.Errorf("command is required")
	}
	cmd := exec.CommandContext(ctx, spec.Command, spec.Args...)
	if dir := strings.TrimSpace(spec.Workdir); dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(cmd.Environ(), spec.Env...)
	return &localCommandProcess{cmd: cmd, stderr: &bytes.Buffer{}}, nil
}

type localCommandProcess struct {
	cmd    *exec.Cmd
	stderr *bytes.Buffer
}

func (p *localCommandProcess) StdinPipe() (io.WriteCloser, error) {
	return p.cmd.StdinPipe()
}

func (p *localCommandProcess) StdoutPipe() (io.ReadCloser, error) {
	return p.cmd.StdoutPipe()
}

func (p *localCommandProcess) StderrText() string {
	if p == nil || p.stderr == nil {
		return ""
	}
	return p.stderr.String()
}

func (p *localCommandProcess) Start() error {
	p.cmd.Stderr = p.stderr
	return p.cmd.Start()
}

func (p *localCommandProcess) Wait() error {
	return p.cmd.Wait()
}

func (p *localCommandProcess) Kill() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}
