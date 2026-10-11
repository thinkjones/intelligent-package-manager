package testtool

import (
	"fmt"
	"strings"
	"testing"
)

// fakeTB records a skip or a failure instead of stopping the test.
type fakeTB struct {
	testing.TB
	skipped, failed string
}

func (f *fakeTB) Helper()                        {}
func (f *fakeTB) Skipf(format string, a ...any)  { f.skipped = fmt.Sprintf(format, a...) }
func (f *fakeTB) Fatalf(format string, a ...any) { f.failed = fmt.Sprintf(format, a...) }

func TestNeed(t *testing.T) {
	t.Setenv(Strict, "")
	f := &fakeTB{}
	if path := Need(f, "go"); !strings.HasSuffix(path, "go") || f.skipped != "" || f.failed != "" {
		t.Errorf("go is installed: got %q, skipped %q, failed %q", path, f.skipped, f.failed)
	}

	f = &fakeTB{}
	if path := Need(f, "ipm-no-such-tool"); path != "" || f.skipped != "ipm-no-such-tool is not installed" || f.failed != "" {
		t.Errorf("without %s a missing tool skips: got %q, skipped %q, failed %q", Strict, path, f.skipped, f.failed)
	}

	t.Setenv(Strict, "1")
	f = &fakeTB{}
	if Need(f, "ipm-no-such-tool"); f.failed != "ipm-no-such-tool is required when IPM_TEST_STRICT=1" || f.skipped != "" {
		t.Errorf("with %s=1 a missing tool fails: skipped %q, failed %q", Strict, f.skipped, f.failed)
	}
}
