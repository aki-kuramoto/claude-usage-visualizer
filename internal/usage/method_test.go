package usage

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// definitionFile is where the Method constants are defined, relative to the
// module root. It is the only file allowed to spell their values.
var definitionFile = filepath.Join("internal", "usage", "method.go")

// TestMethodLiteralsOnlyInDefinition fails when a method's value is written
// as a string literal anywhere but its constant definition. Code must refer
// to the constant instead, so that the value can be found, and changed, in
// one place.
func TestMethodLiteralsOnlyInDefinition(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	found, err := findLiterals(root, methodValues(), definitionFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range found {
		t.Errorf("%s: write the constant instead of the literal %s", f.pos, f.lit)
	}
}

// TestFindLiteralsCatchesPlanted checks the checker itself, on files written
// here with the values in them.
func TestFindLiteralsCatchesPlanted(t *testing.T) {
	root := t.TempDir()
	value := string(MethodStreamJSONResident)
	q := strconv.Quote(value)
	raw := "`" + value + "`"
	write := func(name, src string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	allowed := filepath.Join("def", "method.go")
	write(allowed, "package def\nconst X = "+q+"\nfunc f() string { return "+q+" }\n")
	write(filepath.Join("other", "use.go"), "package other\nvar y = map[string]int{"+raw+": 1}\nconst Z = "+q+"\n")
	write(filepath.Join("other", "use_test.go"), "package other\nfunc g() string { return "+q+" + \"-x\" }\n")
	write(filepath.Join("other", "fine.go"), "package other\nvar s = "+strconv.Quote(value+"-but-longer")+"\n")
	write(filepath.Join("testdata", "skip.go"), "package skip\nvar s = "+q+"\n")

	found, err := findLiterals(root, []string{value}, allowed)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range found {
		got = append(got, fmt.Sprintf("%s:%d", filepath.ToSlash(f.file), f.pos.Line))
	}
	want := []string{"def/method.go:3", "other/use.go:2", "other/use.go:3", "other/use_test.go:2"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("found %v, want %v", got, want)
	}
}

func methodValues() []string {
	var vs []string
	for _, m := range Methods() {
		vs = append(vs, string(m))
	}
	return vs
}

type literal struct {
	file string // relative to the root
	pos  token.Position
	lit  string
}

// findLiterals walks the Go files under root and returns every string literal
// whose value is one of values, except those inside a const declaration in
// allowedFile (a path relative to root).
func findLiterals(root string, values []string, allowedFile string) ([]literal, error) {
	want := map[string]bool{}
	for _, v := range values {
		want[v] = true
	}
	var found []literal
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		var allowed []ast.Node
		if rel == allowedFile {
			for _, decl := range file.Decls {
				if g, ok := decl.(*ast.GenDecl); ok && g.Tok == token.CONST {
					allowed = append(allowed, g)
				}
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			v, err := strconv.Unquote(lit.Value)
			if err != nil || !want[v] {
				return true
			}
			for _, a := range allowed {
				if a.Pos() <= lit.Pos() && lit.End() <= a.End() {
					return true
				}
			}
			found = append(found, literal{file: rel, pos: fset.Position(lit.Pos()), lit: lit.Value})
			return true
		})
		return nil
	})
	return found, err
}

// moduleRoot finds the directory holding go.mod, starting from the package
// directory the test runs in.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found")
		}
		dir = parent
	}
}
