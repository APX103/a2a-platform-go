package bridge

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"a2a-platform/internal/config"
)

const maxCLIOutputBytes = 1 << 20

func invokeCLI(ctx context.Context, skill *config.SkillInvoke, target *config.BridgeCLITarget, tctx *TemplateContext) (string, error) {
	command := renderString(skill.Command, tctx)
	if command == "" {
		return "", fmt.Errorf("empty command")
	}

	var args []string
	for _, a := range skill.Args {
		args = append(args, renderString(a, tctx))
	}

	shell := "bash"
	if target != nil && target.Shell != "" {
		shell = target.Shell
	}

	timeout := 30 * time.Second
	if skill.Timeout > 0 {
		timeout = time.Duration(skill.Timeout) * time.Millisecond
	} else if target != nil && target.Timeout > 0 {
		timeout = time.Duration(target.Timeout) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Bridge CLI commands are trusted operator configuration. Rendered user input
	// must be passed through args in configs that require strict argument safety.
	cmdArgs := []string{"-c", command}
	if len(args) > 0 {
		cmdArgs[1] = command + ` "$@"`
		cmdArgs = append(cmdArgs, command)
		cmdArgs = append(cmdArgs, args...)
	}
	cmd := exec.CommandContext(ctx, shell, cmdArgs...)
	if target != nil && target.WorkingDir != "" {
		cmd.Dir = target.WorkingDir
	}

	var stdout boundedBuffer
	stdout.Limit = maxCLIOutputBytes
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("command timed out after %s", timeout)
		}
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("command failed: %s", errMsg)
	}

	return strings.TrimSpace(stdout.String()), nil
}

type boundedBuffer struct {
	buf   bytes.Buffer
	Limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Limit <= 0 {
		return len(p), nil
	}
	remaining := b.Limit - b.buf.Len()
	if remaining > 0 {
		if len(p) < remaining {
			remaining = len(p)
		}
		_, _ = b.buf.Write(p[:remaining])
	}
	return len(p), nil
}

func (b *boundedBuffer) String() string {
	return b.buf.String()
}
