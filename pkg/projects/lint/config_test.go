package lint

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	return path
}

func TestLoadConfig(t *testing.T) {
	path := writeConfig(t, "pm-kit.yml", `lint:
  status-field: Stage
  done-statuses:
    - Shipped
  required-fields:
    - Priority
  stale-days: 7
  include-archived: true
  fail-on: warning
  ignore:
    - PM005
  severity:
    PM003: error
`)

	opts, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if opts.StatusField != "Stage" {
		t.Errorf("StatusField = %q, want Stage", opts.StatusField)
	}
	if len(opts.DoneStatuses) != 1 || opts.DoneStatuses[0] != "Shipped" {
		t.Errorf("DoneStatuses = %v, want [Shipped]", opts.DoneStatuses)
	}
	if len(opts.RequiredFields) != 1 || opts.RequiredFields[0] != "Priority" {
		t.Errorf("RequiredFields = %v, want [Priority]", opts.RequiredFields)
	}
	if opts.StaleDays != 7 || !opts.IncludeArchived {
		t.Errorf("StaleDays/IncludeArchived = %d/%v, want 7/true", opts.StaleDays, opts.IncludeArchived)
	}
	if opts.FailOn != SeverityWarning {
		t.Errorf("FailOn = %q, want warning", opts.FailOn)
	}
	if len(opts.Ignore) != 1 || opts.Ignore[0] != "PM005" {
		t.Errorf("Ignore = %v, want [PM005]", opts.Ignore)
	}
	if opts.Severity["PM003"] != SeverityError {
		t.Errorf("Severity[PM003] = %q, want error", opts.Severity["PM003"])
	}
}

func TestLoadConfigRejectsUnknownKeys(t *testing.T) {
	path := writeConfig(t, "pm-kit.yml", "lint:\n  stale_days: 7\n")
	if _, err := LoadConfig(path); err == nil {
		t.Error("LoadConfig should reject unknown keys so that typos are not silently ignored")
	}
}

func TestLoadConfigEmptyFile(t *testing.T) {
	path := writeConfig(t, "pm-kit.yml", "")
	opts, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if opts.StatusField != "" {
		t.Errorf("StatusField = %q, want empty so that defaults apply", opts.StatusField)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	if _, err := LoadConfig(filepath.Join(t.TempDir(), "missing.yml")); err == nil {
		t.Error("LoadConfig should fail when the file given explicitly does not exist")
	}
}

func TestFindConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".github"), 0o755); err != nil {
		t.Fatalf("failed to create .github: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".github", "pm-kit.yml"), []byte("lint:\n  stale-days: 3\n"), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	t.Chdir(dir)

	path, opts, err := FindConfig()
	if err != nil {
		t.Fatalf("FindConfig returned error: %v", err)
	}
	if path != ".github/pm-kit.yml" {
		t.Errorf("path = %q, want .github/pm-kit.yml", path)
	}
	if opts == nil || opts.StaleDays != 3 {
		t.Errorf("opts = %+v, want StaleDays 3", opts)
	}
}

func TestFindConfigWithoutConfigFile(t *testing.T) {
	t.Chdir(t.TempDir())

	path, opts, err := FindConfig()
	if err != nil {
		t.Fatalf("FindConfig returned error: %v", err)
	}
	if path != "" || opts != nil {
		t.Errorf("FindConfig = %q/%+v, want empty results", path, opts)
	}
}
