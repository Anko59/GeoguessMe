package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot walks up from the package directory (tools/quality/archcheck) to
// the repository root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(dir)))
}

// writeFixture creates a backend/ subtree under a temp directory and returns
// the temp directory as the fixture repository root.
func writeFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func violationsByRule(vs []violation, rule string) []violation {
	var out []violation
	for _, v := range vs {
		if v.rule == rule {
			out = append(out, v)
		}
	}
	return out
}

func TestFindRepoRootFromNestedDirectory(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "tools", "quality", "archcheck")
	if err := os.MkdirAll(filepath.Join(root, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := findRepoRoot(nested)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("findRepoRoot() = %q, want %q", got, root)
	}
}

func TestFindRepoRootFailsClosed(t *testing.T) {
	if _, err := findRepoRoot(t.TempDir()); err == nil {
		t.Fatal("findRepoRoot() succeeded without repository markers")
	}
}

// Clean fixture: the real repository must pass every rule.
func TestRealRepositoryClean(t *testing.T) {
	vs := checkRepo(repoRoot(t))
	if len(vs) != 0 {
		for _, v := range vs {
			t.Errorf("unexpected violation: %s", v)
		}
	}
}

// Mutable globals include both make() values and composite literals. Maps,
// slices, arrays, and structs all remain writable at package scope even when
// they are initialized as table-like literals.
func TestMutableGlobalDetected(t *testing.T) {
	root := writeFixture(t, map[string]string{
		"backend/internal/foo/evil.go": strings.Join([]string{
			"package foo",
			"var made = make(map[string]int)",
			"var mapped = map[string]int{\"a\": 1}",
			"var sliced = []string{\"a\"}",
			"var arrayed = [...]int{1}",
			"var structured = struct{ value int }{value: 1}",
		}, "\n") + "\n",
	})
	vs := violationsByRule(checkRepo(root), "mutable-global")
	if len(vs) != 5 {
		t.Fatalf("expected 5 mutable-global violations, got %d: %v", len(vs), vs)
	}
	for _, name := range []string{"made", "mapped", "sliced", "arrayed", "structured"} {
		found := false
		for _, violation := range vs {
			if strings.Contains(violation.msg, name) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("violations do not name %s: %v", name, vs)
		}
	}
}

// The allowlist is explicit: a var listed there passes, a near-identical one
// that is not listed still fails.
func TestMutableGlobalAllowlistHonored(t *testing.T) {
	content := "package middleware\n\nvar limiter = &rateLimiter{}\n"
	root := writeFixture(t, map[string]string{
		"backend/internal/middleware/rate_limit.go":         content,
		"backend/internal/middleware/other.go":              "package middleware\n\nvar other = &rateLimiter{}\n",
		"tools/quality/archcheck/mutable-globals.allowlist": "backend/internal/middleware/rate_limit.go:limiter\n",
	})
	vs := violationsByRule(checkRepo(root), "mutable-global")
	if len(vs) != 1 {
		t.Fatalf("expected 1 mutable-global violation, got %d: %v", len(vs), vs)
	}
	if !strings.Contains(vs[0].path, "other.go") || !strings.Contains(vs[0].msg, "other") {
		t.Fatalf("allowlist should exempt only limiter: %s", vs[0])
	}
}

// Sentinel errors, byte-string values, compile-time assertions, and embed.FS
// values are exempt by construction.
func TestExemptPatternsPass(t *testing.T) {
	root := writeFixture(t, map[string]string{
		"backend/internal/repository/things/errors.go": strings.Join([]string{
			"package things",
			"",
			"import \"errors\"",
			"",
			"var ErrNotFound = errors.New(\"not found\")",
			"var info = []byte(\"Content-Encoding: aes128gcm\\x00\")",
			"var _ interface{} = nil",
			"var manifest embed.FS",
		}, "\n") + "\n",
	})
	vs := violationsByRule(checkRepo(root), "mutable-global")
	if len(vs) != 0 {
		t.Fatalf("expected no mutable-global violations, got %d: %v", len(vs), vs)
	}
}

// SQL in a handler: an import of database/sql must be flagged.
func TestSQLImportInHandlerDetected(t *testing.T) {
	root := writeFixture(t, map[string]string{
		"backend/handlers/evil.go": "package handlers\n\nimport \"database/sql\"\n\nvar _ = sql.ErrNoRows\n",
	})
	vs := violationsByRule(checkRepo(root), "sql-in-handlers")
	if len(vs) != 1 {
		t.Fatalf("expected 1 sql-in-handlers violation, got %d: %v", len(vs), vs)
	}
	if !strings.Contains(vs[0].msg, "database/sql") {
		t.Fatalf("violation does not name the import: %s", vs[0])
	}
}

// SQL string literal in a handler must be flagged.
func TestSQLLiteralInHandlerDetected(t *testing.T) {
	root := writeFixture(t, map[string]string{
		"backend/handlers/evil.go": "package handlers\n\nvar query = \"SELECT * FROM users\"\n",
	})
	vs := violationsByRule(checkRepo(root), "sql-in-handlers")
	if len(vs) != 1 {
		t.Fatalf("expected 1 sql-in-handlers violation, got %d: %v", len(vs), vs)
	}
}

// SQL outside handlers (repository layer) is allowed.
func TestSQLInRepositoryAllowed(t *testing.T) {
	root := writeFixture(t, map[string]string{
		"backend/internal/repository/users.go": "package repository\n\nvar q = \"SELECT * FROM users\"\n",
	})
	vs := violationsByRule(checkRepo(root), "sql-in-handlers")
	if len(vs) != 0 {
		t.Fatalf("expected no sql-in-handlers violations, got %d: %v", len(vs), vs)
	}
}

// A direct environment read outside backend/internal/config is flagged.
func TestEnvReadOutsideConfigDetected(t *testing.T) {
	root := writeFixture(t, map[string]string{
		"backend/internal/foo/evil.go": "package foo\n\nimport \"os\"\n\nvar _ = os.Getenv(\"X\")\n",
	})
	vs := violationsByRule(checkRepo(root), "env-read")
	if len(vs) != 1 {
		t.Fatalf("expected 1 env-read violation, got %d: %v", len(vs), vs)
	}
	if !strings.Contains(vs[0].msg, "backend/internal/config") {
		t.Fatalf("violation does not name the allowlisted root: %s", vs[0])
	}
}

// Environment reads inside backend/internal/config are the allowlisted reader.
func TestEnvReadInConfigAllowed(t *testing.T) {
	root := writeFixture(t, map[string]string{
		"backend/internal/config/config.go": "package config\n\nimport \"os\"\n\nvar _ = os.Getenv(\"X\")\n",
	})
	vs := violationsByRule(checkRepo(root), "env-read")
	if len(vs) != 0 {
		t.Fatalf("expected no env-read violations, got %d: %v", len(vs), vs)
	}
}

// os.LookupEnv and syscall.Getenv are equally covered by the env-read rule.
func TestEnvReadVariantsDetected(t *testing.T) {
	root := writeFixture(t, map[string]string{
		"backend/internal/foo/a.go": "package foo\n\nimport \"os\"\n\nvar _ = os.LookupEnv(\"X\")\n",
		"backend/internal/foo/b.go": "package foo\n\nimport \"syscall\"\n\nvar _ = syscall.Getenv(\"X\")\n",
	})
	vs := violationsByRule(checkRepo(root), "env-read")
	if len(vs) != 2 {
		t.Fatalf("expected 2 env-read violations, got %d: %v", len(vs), vs)
	}
}
