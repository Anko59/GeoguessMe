// Command archcheck enforces the durable production-application architecture
// rules of the GeoGuessMe backend. It is deliberately narrow and deterministic:
// the rules below are the invariants the GA refactor established, and every
// rule has an explicit allowlist plus regression fixtures (see main_test.go).
//
// Rules (all scoped to backend/ production Go files; *_test.go and generated
// *_pb.go files are skipped):
//
//  1. mutable-globals — no production package-level mutable application
//     dependencies. Exempt by construction: `var _` compile-time assertions,
//     embed.FS values, `errors.New`/`fmt.Errorf` sentinel errors, basic
//     literals, byte-string literals, and non-pointer composite literals
//     (read-only tables). Everything else must be listed in
//     tools/quality/archcheck/mutable-globals.allowlist. The only entry today
//     is the mutex-protected rate-limiter singleton.
//  2. sql-in-handlers — HTTP handlers under backend/handlers/ must not import
//     database/sql or pgx and must not contain SQL command string literals.
//  3. env-read — no direct environment reads outside backend/internal/config/
//     (the sole configuration and bootstrap reader).
//
// Contract currentness (make openapi-check) and agent/documentation reference
// validity (make lint-docs) are enforced by their own tools and are not
// duplicated here; make archcheck and the preflight/quality gates run all of
// them together.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	allowlistPath = "tools/quality/archcheck/mutable-globals.allowlist"
	configRoot    = "backend/internal/config"
	handlersRoot  = "backend/handlers"
)

type violation struct {
	rule string
	path string
	msg  string
}

func (v violation) String() string {
	return fmt.Sprintf("%s: %s: %s", v.rule, v.path, v.msg)
}

type checker struct {
	allowlist  map[string]bool
	violations []violation
}

func (c *checker) add(rule, path, msg string) {
	c.violations = append(c.violations, violation{rule: rule, path: path, msg: msg})
}

func main() {
	root := os.Getenv("ARCHCHECK_REPO_ROOT")
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		root = wd
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	vs := checkRepo(abs)
	sort.Slice(vs, func(i, j int) bool { return vs[i].String() < vs[j].String() })
	for _, v := range vs {
		fmt.Println(v)
	}
	if len(vs) > 0 {
		fmt.Printf("archcheck FAILED (%d violation(s))\n", len(vs))
		os.Exit(1)
	}
	fmt.Println("archcheck PASSED")
}

func checkRepo(root string) []violation {
	c := &checker{allowlist: loadAllowlist(filepath.Join(root, allowlistPath))}
	backendRoot := filepath.Join(root, "backend")
	info, err := os.Stat(backendRoot)
	if err != nil || !info.IsDir() {
		return nil
	}
	_ = filepath.WalkDir(backendRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		c.checkFile(rel, path)
		return nil
	})
	return c.violations
}

func skipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "dist", "coverage",
		"test-results", "playwright-report", "blob-report", ".pi", ".pi-subagents":
		return true
	}
	return false
}

func loadAllowlist(path string) map[string]bool {
	allowed := make(map[string]bool)
	data, err := os.ReadFile(path)
	if err != nil {
		return allowed
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		allowed[line] = true
	}
	return allowed
}

func (c *checker) checkFile(rel, path string) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		c.add("parse-error", rel, "failed to parse Go source: "+err.Error())
		return
	}
	if strings.HasPrefix(rel, handlersRoot+"/") {
		c.checkSQL(rel, f)
	}
	c.checkGlobals(rel, f)
	c.checkEnv(rel, f)
}

func (c *checker) checkGlobals(rel string, f *ast.File) {
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				if exemptVar(name.Name, vs.Type, firstExpr(vs.Values)) {
					continue
				}
				if c.allowlist[rel+":"+name.Name] {
					continue
				}
				c.add("mutable-global", rel, "package-level var "+name.Name+" must move to the composition root or be allowlisted")
			}
		}
	}
}

func firstExpr(exprs []ast.Expr) ast.Expr {
	if len(exprs) == 0 {
		return nil
	}
	return exprs[0]
}

// exemptVar classifies package-level vars that cannot hold mutable
// application-dependency state.
func exemptVar(name string, typ ast.Expr, val ast.Expr) bool {
	if name == "_" {
		return true
	}
	if isEmbedFS(typ) {
		return true
	}
	switch v := val.(type) {
	case *ast.CallExpr:
		return isSentinelError(v) || isByteStringLiteral(v)
	case *ast.BasicLit:
		return true
	case *ast.CompositeLit:
		return true // value (non-pointer) composite literal: read-only table
	}
	return false
}

func isEmbedFS(t ast.Expr) bool {
	switch v := t.(type) {
	case *ast.Ident:
		return v.Name == "FS"
	case *ast.SelectorExpr:
		ident, ok := v.X.(*ast.Ident)
		return ok && ident.Name == "embed" && v.Sel.Name == "FS"
	}
	return false
}

func isSentinelError(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return (pkg.Name == "errors" && sel.Sel.Name == "New") ||
		(pkg.Name == "fmt" && sel.Sel.Name == "Errorf")
}

// isByteStringLiteral reports `[]byte("...")`: an immutable byte-string value.
func isByteStringLiteral(call *ast.CallExpr) bool {
	arr, ok := call.Fun.(*ast.ArrayType)
	if !ok {
		return false
	}
	elt, ok := arr.Elt.(*ast.Ident)
	if !ok || elt.Name != "byte" {
		return false
	}
	if len(call.Args) != 1 {
		return false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	return ok && lit.Kind == token.STRING
}

func (c *checker) checkSQL(rel string, f *ast.File) {
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		if p == "database/sql" || strings.HasPrefix(p, "github.com/jackc/pgx") {
			c.add("sql-in-handlers", rel, "handler imports "+p)
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		lower := strings.ToLower(strings.TrimSpace(strings.Trim(lit.Value, `"`)))
		if strings.HasPrefix(lower, "select ") || strings.HasPrefix(lower, "insert into ") ||
			strings.HasPrefix(lower, "delete from ") {
			c.add("sql-in-handlers", rel, "string literal looks like a SQL statement: "+lit.Value)
		}
		return true
	})
}

func (c *checker) checkEnv(rel string, f *ast.File) {
	inConfig := rel == configRoot || strings.HasPrefix(rel, configRoot+"/")
	if inConfig {
		return
	}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if (pkg.Name == "os" && (sel.Sel.Name == "Getenv" || sel.Sel.Name == "LookupEnv")) ||
			(pkg.Name == "syscall" && sel.Sel.Name == "Getenv") {
			c.add("env-read", rel, "direct environment read outside "+configRoot)
		}
		return true
	})
}
