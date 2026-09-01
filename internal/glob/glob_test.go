package glob

import "testing"

func TestHasWildcard(t *testing.T) {
	tests := []struct {
		pattern string
		want    bool
	}{
		{"**/*.go", true},
		{"file?.txt", true},
		{"[a-z]*.go", true},
		{"*.{go,md}", true},
		{"cmd", false},
		{"deploy/.env.example", false},
	}
	for _, tt := range tests {
		if got := HasWildcard(tt.pattern); got != tt.want {
			t.Errorf("HasWildcard(%q) = %v, want %v", tt.pattern, got, tt.want)
		}
	}
}

func TestValidate(t *testing.T) {
	valid := []string{"**/*", "vendor/**", "cmd", "[a-z].txt", "{a,b}/*.go"}
	for _, p := range valid {
		if err := Validate([]string{p}); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", p, err)
		}
	}
	if err := Validate([]string{"[invalid"}); err == nil {
		t.Error("Validate([invalid) should fail: unterminated character class")
	}
}

func TestExcluded_IncludePriority(t *testing.T) {
	includes := []string{"deploy/.env.example"}
	excludes := []string{"deploy/**"}
	if !Excluded(includes, excludes, "deploy/other.txt") {
		t.Error("non-matching include should not save an excluded file")
	}
	if Excluded(includes, excludes, "deploy/.env.example") {
		t.Error("explicit include must win over generic exclude")
	}
}

func TestExcluded_PlainIncludeWins(t *testing.T) {
	includes := []string{"**/*", "keep/secret.env"}
	excludes := []string{"**/*.env"}
	if Excluded(includes, excludes, "keep/secret.env") {
		t.Error("exact include must beat wildcard exclude")
	}
}

// Regression: doublestar only honors ** as a globstar when followed by the
// platform separator. Our slash-normalized patterns/rels must match bare
// files at any depth on every OS. (This used to fail on Windows, where the
// separator is a backslash, silently dropping all root-level files.)
func TestMatchPattern_GlobstarMatchesBareFiles(t *testing.T) {
	if !MatchPattern("**/*", "text.txt") {
		t.Error("**/* must match a bare root file")
	}
	if !MatchPattern("**/*.go", "src/main.go") {
		t.Error("**/*.go must match a nested file")
	}
	if !MatchPattern("**/*", "src/main.go") {
		t.Error("**/* must match a nested file")
	}
	if MatchPattern("**/*.go", "src/main.txt") {
		t.Error("**/*.go must not match wrong extension")
	}
}
