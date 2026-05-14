// Copyright 2025 The Libc Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// Tool for generating assembler libc wrappers.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/types"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
	"modernc.org/goabi0"
)

const (
	// https://github.com/golang/go/issues/41196
	//
	// ^// Code generated .* DO NOT EDIT.$
	generatedFilePrefix = "Code generated"
	generatedFileSuffix = ", DO NOT EDIT."
)

var (
	_ goabi0.Param = (*param)(nil)
	_ goabi0.Slot  = (*slot)(nil)
	_ goabi0.Type  = (*typ)(nil)

	goarch = runtime.GOARCH
	goos   = runtime.GOOS

	pkg      *packages.Package // modernc.org/libc
	wordSize = int64(8)
)

type param struct {
	*typ
	name string
}

func (p *param) Name() string {
	return p.name
}

func (p *param) GetType() goabi0.Type {
	return p.typ
}

type slot struct {
	*param
	offset int64
}

func (s *slot) Offset() int64 {
	return s.offset
}

type typ struct {
	alignof int64
	elem    *typ
	fields  []goabi0.Slot
	len     int64
	sizeof  int64
}

func (t *typ) Alignof() int64 {
	return t.alignof
}

func (t *typ) Kind() goabi0.Kind {
	return goabi0.Kindof(t)
}

func (t *typ) Elem() goabi0.Type {
	return t.elem
}

func (t *typ) Fields() []goabi0.Slot {
	return t.fields
}

func (t *typ) Len() int64 {
	return t.len
}

func (t *typ) Sizeof() int64 {
	return t.sizeof
}

// origin returns caller's short position, skipping skip frames.
func origin(skip int) string {
	pc, fn, fl, _ := runtime.Caller(skip)
	f := runtime.FuncForPC(pc)
	var fns string
	if f != nil {
		fns = f.Name()
		if x := strings.LastIndex(fns, "."); x > 0 {
			fns = fns[x+1:]
		}
		if strings.HasPrefix(fns, "func") {
			num := true
			for _, c := range fns[len("func"):] {
				if c < '0' || c > '9' {
					num = false
					break
				}
			}
			if num {
				return origin(skip + 2)
			}
		}
	}
	return fmt.Sprintf("%s:%d:%s", filepath.Base(fn), fl, fns)
}

// todo prints and return caller's position and an optional message tagged with TODO. Output goes to stderr.
func todo(s string, args ...interface{}) string {
	switch {
	case s == "":
		s = fmt.Sprintf(strings.Repeat("%v ", len(args)), args...)
	default:
		s = fmt.Sprintf(s, args...)
	}
	r := fmt.Sprintf("%s\n\tTODO %s", origin(2), s)
	// fmt.Fprintf(os.Stderr, "%s\n", r)
	// os.Stdout.Sync()
	return r
}

// trc prints and return caller's position and an optional message tagged with TRC. Output goes to stderr.
func trc(s string, args ...interface{}) string {
	switch {
	case s == "":
		s = fmt.Sprintf(strings.Repeat("%v ", len(args)), args...)
	default:
		s = fmt.Sprintf(s, args...)
	}
	r := fmt.Sprintf("%s: TRC %s", origin(2), s)
	fmt.Fprintf(os.Stderr, "%s\n", r)
	os.Stderr.Sync()
	return r
}

func fail(rc int, msg string, args ...any) {
	fmt.Fprintln(os.Stderr, strings.TrimSpace(fmt.Sprintf("FAIL: "+msg, args...)))
	os.Exit(rc)
}

type buf bytes.Buffer

func (b *buf) w(s string, args ...any) {
	fmt.Fprintf((*bytes.Buffer)(b), s, args...)
}

func (b *buf) Write(p []byte) (int, error) {
	return (*bytes.Buffer)(b).Write(p)
}

func main() {
	switch goarch {
	case "386", "arm":
		wordSize = 4
	}
	cfg := &packages.Config{
		Mode: packages.NeedName | // Package name
			packages.NeedFiles | // Go source files for the package
			packages.NeedCompiledGoFiles | // Augmented list of files for LoadFiles mode
			packages.NeedImports | // Direct imports of the package
			packages.NeedDeps | // Transitive dependencies
			packages.NeedTypes | // Go type data ([*types.Package])
			packages.NeedSyntax | // ASTs ([*ast.File])
			packages.NeedTypesInfo | // Type information for expressions ([*types.Info])
			packages.NeedTypesSizes, // Sizes of types (types.Sizes)
	}
	pkgs, err := packages.Load(cfg, "modernc.org/libc")
	if err != nil || len(pkgs) != 1 {
		fail(1, "Failed to load package: %v", err)
	}

	pkg = pkgs[0]
	if len(pkg.Errors) != 0 {
		fail(1, "%v", pkg.Errors)
	}

	if len(pkg.TypeErrors) != 0 {
		fail(1, "%v", pkg.Errors)
	}

	nodes := map[string]*ast.FuncDecl{}
	var names []string
	for _, w := range pkg.Syntax {
		for _, x := range w.Decls {
			switch y := x.(type) {
			case *ast.FuncDecl:
				nm := y.Name.Name
				switch {
				case
					strings.HasPrefix(nm, "X__assert"),
					strings.HasPrefix(nm, "X__builtin"):

					// ok
				case strings.HasPrefix(nm, "X_"):
					continue
				case strings.HasPrefix(nm, "X"):
					// ok
				default:
					continue
				}

				l := y.Type.Params.List
				if len(l) == 0 {
					continue
				}

				switch z := l[0].Type.(type) {
				case *ast.StarExpr:
					switch a := z.X.(type) {
					case *ast.Ident:
						if a.Name != "TLS" {
							continue
						}
					default:
						panic(todo("%T", a))
					}
				default:
					continue
				}

				names = append(names, nm)
				nodes[nm] = y
			}
		}
	}
	sort.Strings(names)
	y := &buf{} // asm_os_arch.go
	a := &buf{} // asm_os_arch.s
	args := strings.Join(os.Args[1:], " ")
	if args != "" {
		args = " " + args
	}
	header := fmt.Sprintf("// %s for %s/%s by '%s%v'%s\n",
		generatedFilePrefix, goos, goarch, filepath.Base(os.Args[0]), args, generatedFileSuffix)
	y.w("%s\npackage libc\n\n", header)
	a.w("%s\n", header)
	a.w("#include \"funcdata.h\"\n")
	a.w("#include \"textflag.h\"\n")
	for _, nm := range names {
		fdn := nodes[nm]
		y.w("//go:noescape\n")
		y.w("func Y%s", nm[1:])
		signature(y, fdn.Type)
		y.w("\n")
		in, out := inout(fdn.Type)
		frame, args, stackIn, stackOut := goabi0.StackLayout(wordSize, in, out)
		a.w("\n// func Y%s", nm[1:])
		signature(a, fdn.Type)
		a.w("\nTEXT ·Y%s(SB),$%v-%v\n", nm[1:], frame, args)
		a.w("\tGO_ARGS\n")
		a.w("\tNO_LOCAL_POINTERS\n")
		for _, v := range stackIn {
			moves, err := goabi0.Cp(wordSize, v)
			if err != nil {
				fail(1, "%v", err)
			}

			for _, w := range moves {
				a.w("\tMOV%s %s+%v(FP), AX\n", suffices[w.Size], w.Name, w.Offset)
				a.w("\tMOV%s AX, %v(SP)\n", suffices[w.Size], w.Offset)
			}
		}
		a.w("\tCALL ·%s(SB)\n", nm)
		for _, v := range stackOut {
			moves, err := goabi0.Cp(wordSize, v)
			if err != nil {
				fail(1, "%v", err)
			}

			for _, w := range moves {
				a.w("\tMOV%s %v(SP), AX\n", suffices[w.Size], w.Offset)
				a.w("\tMOV%s AX, %s+%v(FP)\n", suffices[w.Size], w.Name, w.Offset)
			}
		}
		a.w("\tRET\n")
	}
	if err := os.WriteFile(fmt.Sprintf("asm_%s_%s.go", goos, goarch), (*bytes.Buffer)(y).Bytes(), 0660); err != nil {
		fail(1, "%v", err)
	}

	if err := os.WriteFile(fmt.Sprintf("asm_%s_%s.s", goos, goarch), (*bytes.Buffer)(a).Bytes(), 0660); err != nil {
		fail(1, "%v", err)
	}
}

var suffices = [...]string{
	1: "B",
	2: "W",
	4: "L",
	8: "Q",
}

func inout(n *ast.FuncType) (in, out []goabi0.Param) {
	return params(n.Params, "arg"), params(n.Results, "ret")
}

func params(n *ast.FieldList, defaultNm string) (r []goabi0.Param) {
	if n == nil {
		return nil
	}

	for _, v := range n.List {
		t := typeof(v.Type)
		switch tn := fmt.Sprint(v.Type); tn {
		case "complex64":
			ft := &typ{alignof: 4, sizeof: 4, len: -1}
			t.fields = []goabi0.Slot{
				&slot{offset: 0, param: &param{name: "real", typ: ft}},
				&slot{offset: 4, param: &param{name: "imag", typ: ft}},
			}
		case "complex128":
			ft := &typ{alignof: wordSize, sizeof: 8, len: -1}
			t.fields = []goabi0.Slot{
				&slot{offset: 0, param: &param{name: "real", typ: ft}},
				&slot{offset: 8, param: &param{name: "imag", typ: ft}},
			}
		}
		switch {
		case len(v.Names) == 0:
			r = append(r, &param{typ: t, name: defaultNm})
		default:
			for _, v := range v.Names {
				r = append(r, &param{typ: t, name: rename(v.Name)})
			}
		}
	}
	return r
}

func typeof(t any) (r *typ) {
	switch x := t.(type) {
	case *ast.StarExpr:
		return &typ{alignof: wordSize, sizeof: wordSize, len: -1}
	case *ast.Ident:
		t := pkg.TypesInfo.Types[x].Type.Underlying()
		switch x := t.(type) {
		case *types.Basic:
			return typeof(x)
		case *types.Struct:
			r = &typ{alignof: pkg.TypesSizes.Alignof(t), sizeof: pkg.TypesSizes.Sizeof(t), len: -1}
			var flds []*types.Var
			for i := 0; i < x.NumFields(); i++ {
				f := x.Field(i)
				sz := pkg.TypesSizes.Sizeof(f.Type())
				if sz != 0 {
					flds = append(flds, x.Field(i))
				}
			}
			offs := pkg.TypesSizes.Offsetsof(flds)
			for i, f := range flds {
				r.fields = append(r.fields, &slot{offset: offs[i], param: &param{name: f.Name(), typ: typeof(f.Type())}})
			}
			return r
		default:
			panic(todo("%T", x))
		}
	case *types.Basic:
		return &typ{alignof: pkg.TypesSizes.Alignof(x), sizeof: pkg.TypesSizes.Sizeof(x), len: -1}
	case *types.Alias:
		return typeof(x.Underlying())
	case *types.Array:
		return &typ{alignof: pkg.TypesSizes.Alignof(x), sizeof: pkg.TypesSizes.Sizeof(x), len: x.Len(), elem: typeof(x.Elem())}
	default:
		panic(todo("%T", x))
	}
}

func signature(b *buf, n *ast.FuncType) {
	fieldList(b, n.Params, true)
	if n.Results != nil {
		b.w(" ")
		fieldList(b, n.Results, false)
	}
}

func rename(nm string) string {
	switch nm {
	case "g":
		return "g_"
	default:
		return nm
	}
}

func fieldList(b *buf, n *ast.FieldList, parens bool) {
	if n == nil {
		return
	}

	if !parens {
		parens = mustParens(n)
	}
	if parens {
		b.w("(")
		defer b.w(")")
	}
	for i, v := range n.List {
		if i != 0 {
			b.w(", ")
		}
		for j, w := range v.Names {
			if j != 0 {
				b.w(", ")
			}
			b.w("%s", rename(w.Name))
		}
		if len(v.Names) != 0 {
			b.w(" ")
		}
		printer.Fprint(b, pkg.Fset, v.Type)
	}
}

func mustParens(l *ast.FieldList) bool {
	if l == nil {
		return false
	}

	switch len(l.List) {
	case 0:
		return false
	case 1:
		return len(l.List[0].Names) != 0
	default:
		return true
	}
}
