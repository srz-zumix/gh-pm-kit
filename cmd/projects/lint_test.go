package projects

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-pm-kit/pkg/projects/lint"
)

// TestResolveLintOptionsDefaultsFailOn verifies that, with no config file and no --fail-on
// flag, the resolved fail-on threshold falls back to the documented default instead of the
// empty string (which would rank as 0 and make any finding trigger a non-zero exit).
func TestResolveLintOptionsDefaultsFailOn(t *testing.T) {
	cmd := &cobra.Command{}
	resolved, err := resolveLintOptions(cmd, "", "", lint.Options{})
	if err != nil {
		t.Fatalf("resolveLintOptions returned error: %v", err)
	}
	if resolved.FailOn != lint.DefaultFailOn {
		t.Fatalf("FailOn = %q, want %q", resolved.FailOn, lint.DefaultFailOn)
	}
}

// TestResolveLintOptionsCanonicalizesConfigFailOn verifies that a mixed-case fail-on value
// from the config file is canonicalized so it ranks correctly in ShouldFail.
func TestResolveLintOptionsCanonicalizesConfigFailOn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pm-kit.yml")
	if err := os.WriteFile(path, []byte("lint:\n  fail-on: Warning\n"), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	cmd := &cobra.Command{}
	resolved, err := resolveLintOptions(cmd, path, "", lint.Options{})
	if err != nil {
		t.Fatalf("resolveLintOptions returned error: %v", err)
	}
	if resolved.FailOn != lint.SeverityWarning {
		t.Fatalf("FailOn = %q, want %q", resolved.FailOn, lint.SeverityWarning)
	}
}
