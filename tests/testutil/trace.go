package testutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kuetix/logger"
)

// Step narrates a test step. Output shows only with `go test -v` or when the
// test fails — exactly when you want to see what ran.
func Step(t testing.TB, format string, args ...interface{}) {
	t.Helper()
	t.Logf("▶ "+format, args...)
}

// result narrates the outcome of a step with a ✓ / ✗ marker and timing.
func result(t testing.TB, ok bool, took time.Duration, format string, args ...interface{}) {
	t.Helper()
	mark := "✓"
	if !ok {
		mark = "✗"
	}
	t.Logf("%s %s  (%s)", mark, fmt.Sprintf(format, args...), took.Round(time.Millisecond))
}

// preview renders any value as a short one-or-few-line string for test logs.
func preview(v interface{}) string {
	if v == nil {
		return "<nil>"
	}
	if s, ok := v.(string); ok {
		return truncate(s, 600)
	}
	b, err := json.MarshalIndent(v, "  ", "  ")
	if err != nil {
		return truncate(fmt.Sprintf("%#v", v), 600)
	}
	return truncate(string(b), 600)
}

func truncate(s string, n int) string {
	s = strings.TrimRight(s, "\n")
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("\n  … (%d bytes total, truncated)", len(s))
}

// captureEngineLog redirects the Kuetix logger's stdout sink into an in-memory
// buffer for the duration of one engine run, and returns a dump func. Call
// dump(force) exactly once afterwards: with force=true (or under `-v`) every
// captured line is attached to the test log; otherwise only WARN/ERROR/FATAL
// lines are — so a passing quiet run stays quiet but a surprising warning
// still surfaces.
//
// It works by swapping the logger's exported StdOut sink (which the engine
// re-points to on every bootstrap) for a pipe we drain concurrently.
func captureEngineLog(t testing.TB) (dump func(force bool)) {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		return func(bool) {}
	}

	origStdOut := logger.StdOut
	logger.StdOut = w
	logger.SetOutput(w)

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	dumped := false
	return func(force bool) {
		t.Helper()
		if dumped {
			return
		}
		dumped = true

		logger.StdOut = origStdOut
		logger.SetOutput(origStdOut)
		_ = w.Close()
		<-done
		_ = r.Close()

		text := strings.TrimRight(buf.String(), "\n")
		if text == "" {
			return
		}
		lines := strings.Split(text, "\n")
		if force || testing.Verbose() {
			t.Logf("engine log (%d lines):\n%s", len(lines), indent(text))
			return
		}
		var notable []string
		for _, ln := range lines {
			if strings.Contains(ln, `"level":"WARN"`) || strings.Contains(ln, `"level":"ERROR"`) || strings.Contains(ln, `"level":"FATAL"`) {
				notable = append(notable, ln)
			}
		}
		if len(notable) > 0 {
			t.Logf("engine log — %d warning/error line(s):\n%s", len(notable), indent(strings.Join(notable, "\n")))
		}
	}
}

func indent(s string) string {
	return "    " + strings.ReplaceAll(s, "\n", "\n    ")
}
