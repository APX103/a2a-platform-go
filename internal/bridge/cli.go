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

	fullCmd := command
	if len(args) > 0 {
		fullCmd += " " + strings.Join(args, " ")
	}

	cmd := exec.CommandContext(ctx, shell, "-c", fullCmd)
	if target != nil && target.WorkingDir != "" {
		cmd.Dir = target.WorkingDir
	}

	var stdout, stderr bytes.Buffer
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
