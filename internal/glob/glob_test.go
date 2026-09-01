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
