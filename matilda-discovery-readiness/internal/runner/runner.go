package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	StatusPass = "PASS"
	StatusFail = "FAIL"
	StatusSkip = "SKIP"
)

type Result struct {
	Name   string
	Status string
	Detail string
}

type Check struct {
	Run func() Result
}

func CommandCheck(name string, command string, args ...string) Check {
	return Check{Run: func() Result {
		out, err := RunCapture("", command, args...)
		if err != nil {
			return Result{Name: name, Status: StatusFail, Detail: err.Error()}
		}
		return Result{Name: name, Status: StatusPass, Detail: firstLine(out)}
	}}
}

func FileCheck(name string, path string) Check {
	return Check{Run: func() Result {
		info, err := os.Stat(path)
		if err != nil {
			return Result{Name: name, Status: StatusFail, Detail: "missing: " + path}
		}
		if info.IsDir() {
			return Result{Name: name, Status: StatusFail, Detail: "expected file, found directory"}
		}
		return Result{Name: name, Status: StatusPass, Detail: path}
	}}
}

func DirCheck(name string, path string) Check {
	return Check{Run: func() Result {
		info, err := os.Stat(path)
		if err != nil {
			return Result{Name: name, Status: StatusFail, Detail: "missing: " + path}
		}
		if !info.IsDir() {
			return Result{Name: name, Status: StatusFail, Detail: "expected directory"}
		}
		return Result{Name: name, Status: StatusPass, Detail: path}
	}}
}

func RunCapture(workdir string, command string, args ...string) (string, error) {
	cmd := exec.Command(command, args...)
	if workdir != "" {
		cmd.Dir = workdir
	}
	applyLocalAnsibleTemp(cmd, workdir)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		if errOut.Len() > 0 {
			return out.String(), fmt.Errorf("%w: %s", err, errOut.String())
		}
		return out.String(), err
	}
	return out.String(), nil
}

func RunStream(workdir string, stdout io.Writer, stderr io.Writer, command string, args ...string) error {
	return RunStreamContext(context.Background(), workdir, stdout, stderr, command, args...)
}

func RunStreamContext(ctx context.Context, workdir string, stdout io.Writer, stderr io.Writer, command string, args ...string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = workdir
	applyLocalAnsibleTemp(cmd, workdir)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	return nil
}

func applyLocalAnsibleTemp(cmd *exec.Cmd, workdir string) {
	if workdir == "" {
		return
	}
	localTemp := filepath.Join(workdir, ".ansible", "tmp")
	controlPathDir := filepath.Join(shortTempRoot(), "matilda-prep-cp")
	_ = os.MkdirAll(localTemp, 0700)
	_ = os.MkdirAll(controlPathDir, 0700)
	cmd.Env = append(os.Environ(),
		"ANSIBLE_CONFIG="+filepath.Join(workdir, "ansible", "ansible.cfg"),
		"ANSIBLE_LOCAL_TEMP="+localTemp,
		"ANSIBLE_SSH_CONTROL_PATH_DIR="+controlPathDir,
	)
}

func shortTempRoot() string {
	for _, candidate := range []string{"/tmp", "/private/tmp"} {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return os.TempDir()
}

func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' || r == '\r' {
			return s[:i]
		}
	}
	return s
}
