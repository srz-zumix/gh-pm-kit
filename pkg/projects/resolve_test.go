package projects

import "testing"

func TestResolveProject(t *testing.T) {
	cases := []struct {
		name       string
		arg        string
		ownerFlag  string
		wantHost   string
		wantOwner  string
		wantNumber int
		wantErr    bool
	}{
		{
			name:       "number with explicit host owner flag",
			arg:        "5",
			ownerFlag:  "github.com/octocat",
			wantHost:   "github.com",
			wantOwner:  "octocat",
			wantNumber: 5,
		},
		{
			name:       "url owner takes precedence over flag",
			arg:        "https://github.com/orgs/url-org/projects/7",
			ownerFlag:  "github.com/flag-owner",
			wantHost:   "github.com",
			wantOwner:  "url-org",
			wantNumber: 7,
		},
		{
			name:       "ghes url extracts host and owner",
			arg:        "https://ghes.example.com/orgs/ent-org/projects/3",
			ownerFlag:  "",
			wantHost:   "ghes.example.com",
			wantOwner:  "ent-org",
			wantNumber: 3,
		},
		{
			name:      "invalid project argument returns error",
			arg:       "not-a-project",
			ownerFlag: "github.com/octocat",
			wantErr:   true,
		},
		{
			name:      "invalid owner returns error",
			arg:       "5",
			ownerFlag: "github.com/octocat/extra",
			wantErr:   true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, number, err := ResolveProject(tc.arg, tc.ownerFlag)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got nil")
				}
				if number != 0 || repo.Host != "" || repo.Owner != "" || repo.Name != "" {
					t.Fatalf("expected zero values on error, got repo=%+v number=%d", repo, number)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if repo.Host != tc.wantHost || repo.Owner != tc.wantOwner || repo.Name != "" {
				t.Fatalf("repo mismatch: got %+v, want host=%q owner=%q name=\"\"", repo, tc.wantHost, tc.wantOwner)
			}
			if number != tc.wantNumber {
				t.Fatalf("number mismatch: got %d, want %d", number, tc.wantNumber)
			}
		})
	}
}

// TestResolveProjectFallsBackToCurrentRepositoryOwner verifies that, with no URL and no
// owner flag, the owner is resolved from the current repository (here forced via GH_REPO).
func TestResolveProjectFallsBackToCurrentRepositoryOwner(t *testing.T) {
	t.Setenv("GH_REPO", "github.com/octocat/example")
	repo, number, err := ResolveProject("5", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.Host != "github.com" || repo.Owner != "octocat" {
		t.Fatalf("repo mismatch: got %+v, want host=github.com owner=octocat", repo)
	}
	if number != 5 {
		t.Fatalf("number mismatch: got %d, want 5", number)
	}
}
