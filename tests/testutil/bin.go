package testutil

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	binOnce sync.Once
	binPath string
	binErr  error
)

// Binary builds ./cmd/cli once per test process and returns the path to the
// compiled `kue` binary. Subsequent calls reuse it.
func Binary(t *testing.T) string {
	t.Helper()
	binOnce.Do(func() {
		root := ModuleRoot(t)
		dir, err := os.MkdirTemp("", "kue-e2e-bin-")
		if err != nil {
			binErr = err
			return
		}
		binPath = filepath.Join(dir, "kue")
		args := []string{"build", "-o", binPath, "./cmd/cli"}
		if _, err := os.Stat(filepath.Join(root, "vendor", "modules.txt")); err == nil {
			args = append([]string{"build", "-mod=vendor"}, args[1:]...)
		}
		cmd := exec.Command("go", args...)
		cmd.Dir = root
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			binErr = &buildError{err: err, output: stderr.String()}
		}
	})
	if binErr != nil {
		t.Fatalf("build kue binary: %v", binErr)
	}
	return binPath
}

type buildError struct {
	err    error
	output string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.output }

// CLIResult is the outcome of one subprocess invocation.
type CLIResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Combined returns stdout+stderr, convenient for substring assertions.
func (r CLIResult) Combined() string { return r.Stdout + r.Stderr }

// JSONStdout returns stdout with the engine's `{"level":...}` log lines
// removed, from the first remaining '{' onward — the machine-readable payload
// of a `--json` invocation.
func (r CLIResult) JSONStdout() string {
	var b strings.Builder
	for _, line := range strings.Split(r.Stdout, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), `{"level":`) {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	s := b.String()
	if i := strings.IndexByte(s, '{'); i >= 0 {
		return s[i:]
	}
	return s
}

// RunCLI invokes the built binary with args. env entries ("KEY=value") are
// appended to the current environment. dir, when non-empty, is the working
// directory.
//
// The invocation, exit code, timing, and (under `-v` or on failure) the full
// stdout/stderr are narrated to the test log.
func RunCLI(t *testing.T, dir string, env []string, args ...string) CLIResult {
	t.Helper()
	bin := Binary(t)

	line := "$ kue " + strings.Join(args, " ")
	extra := []string{}
	if dir != "" {
		extra = append(extra, "cwd="+shortDir(dir))
	}
	for _, e := range env {
		if k, _, ok := strings.Cut(e, "="); ok {
			extra = append(extra, k)
		}
	}
	if len(extra) > 0 {
		line += "    [" + strings.Join(extra, " ") + "]"
	}
	Step(t, "%s", line)

	cmd := exec.Command(bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), env...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	start := time.Now()
	err := cmd.Run()
	took := time.Since(start)

	res := CLIResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exit.ExitCode()
		} else {
			t.Fatalf("run %s %s: %v", bin, strings.Join(args, " "), err)
		}
	}

	result(t, res.ExitCode == 0, took, "exit=%d  (stdout %dB, stderr %dB)", res.ExitCode, len(res.Stdout), len(res.Stderr))

	dumpOutput := func() {
		t.Helper()
		if out := strings.TrimRight(res.Stdout, "\n"); out != "" {
			t.Logf("  stdout:\n%s", indent(out))
		}
		if er := strings.TrimRight(res.Stderr, "\n"); er != "" {
			t.Logf("  stderr:\n%s", indent(er))
		}
	}
	if testing.Verbose() {
		dumpOutput()
	} else {
		t.Cleanup(func() {
			if t.Failed() {
				dumpOutput()
			}
		})
	}
	return res
}

func shortDir(p string) string {
	if i := strings.LastIndex(p, "/tests/"); i >= 0 {
		return p[i+1:]
	}
	if h, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, h) {
		return "~" + p[len(h):]
	}
	return p
}
