package main

// This test reads the Windows sources, so it runs on every OS.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// wrappers are the functions that take pointers as uintptr arguments. Each
// must be marked go:uintptrescapes (see call in win32_windows.go).
var wrappers = map[string]bool{"call": true, "sendMessage": true, "postMessage": true}

// pointerExceptions are the functions allowed to turn a pointer into a
// uintptr outside a call's argument list, each with the reason it is safe.
var pointerExceptions = map[string]string{
	// TASKDIALOGCONFIG is built byte by byte, pointers included; the
	// function keeps every one of them alive with runtime.KeepAlive until
	// TaskDialogIndirect returns, and all of them are on the heap.
	"tray_windows.go:askClose": "kept alive by runtime.KeepAlive",
	// lParam already is a pointer from Windows, to memory Go does not own.
	"win32_windows.go:rectAt": "not Go's memory",
}

// TestPointersPassedToWin32 checks that every pointer handed to Windows is
// converted to uintptr right in the argument list of a wrapper or of
// LazyProc.Call. Converted anywhere else, the compiler does not know it is
// a pointer: what it points to may stay on the stack, which moves when the
// stack grows, or be freed during the call. That once left the window
// without keyboard shortcuts, the accelerator table having been read from
// where the stack used to be.
func TestPointersPassedToWin32(t *testing.T) {
	files, err := filepath.Glob("*_windows.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no Windows sources: %v", err)
	}
	fset := token.NewFileSet()
	marked := map[string]bool{}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if fn.Recv == nil && wrappers[fn.Name.Name] {
				marked[fn.Name.Name] = fn.Doc != nil && strings.Contains(fn.Doc.Text()+directives(fn.Doc), "go:uintptrescapes")
			}
			where := name + ":" + fn.Name.Name
			if _, ok := pointerExceptions[where]; ok {
				continue
			}
			var stack []ast.Node
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if n == nil {
					stack = stack[:len(stack)-1]
					return true
				}
				if isPointerToUintptr(n) && !inWrapperArgs(n, stack) {
					t.Errorf("%s: a pointer is turned into a uintptr outside the arguments of call, sendMessage, postMessage or LazyProc.Call", fset.Position(n.Pos()))
				}
				stack = append(stack, n)
				return true
			})
		}
	}
	for w := range wrappers {
		if !marked[w] {
			t.Errorf("%s is not marked //go:uintptrescapes", w)
		}
	}
}

// directives returns the //go: lines of a doc comment, which Text leaves out.
func directives(g *ast.CommentGroup) string {
	var b strings.Builder
	for _, c := range g.List {
		if strings.HasPrefix(c.Text, "//go:") {
			b.WriteString(c.Text + "\n")
		}
	}
	return b.String()
}

// isPointerToUintptr reports whether n is uintptr(unsafe.Pointer(...)).
func isPointerToUintptr(n ast.Node) bool {
	c, ok := n.(*ast.CallExpr)
	if !ok || len(c.Args) != 1 {
		return false
	}
	if id, ok := c.Fun.(*ast.Ident); !ok || id.Name != "uintptr" {
		return false
	}
	inner, ok := c.Args[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := inner.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Pointer" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "unsafe"
}

// inWrapperArgs reports whether n, with its ancestors in stack, is itself
// an argument of a wrapper or of a .Call method.
func inWrapperArgs(n ast.Node, stack []ast.Node) bool {
	if len(stack) == 0 {
		return false
	}
	parent, ok := stack[len(stack)-1].(*ast.CallExpr)
	if !ok {
		return false
	}
	isArg := false
	for _, a := range parent.Args {
		if a == n {
			isArg = true
		}
	}
	if !isArg {
		return false
	}
	switch f := parent.Fun.(type) {
	case *ast.Ident:
		return wrappers[f.Name]
	case *ast.SelectorExpr:
		return f.Sel.Name == "Call"
	}
	return false
}
