package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/ioutil"
	l "log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/vburenin/ifacemaker/maker"
)

var log = l.New(os.Stderr, "", l.Lshortfile)

// Field ...
type Field struct {
	Name string
	Type string
}

// Method ...
type Method struct {
	Name    string
	Params  []*Field
	Results []*Field
}

// Struct ...
type Struct struct {
	Name    string
	Methods []*Method
	Fields  []*Field
	Parent  *ast.StructType
}

// Function ...
type Function struct {
	Name       string
	ImportPath string
	Parameters []*Field
	Results    []*Field
}

// Package ...
type Package struct {
	Name       string
	Structs    []*Struct
	Functions  []*Function
	ImportPath string
}

func main() {
	out := flag.String("output", "", "Output dir")
	in := flag.String("input", "", "Package to make testable")
	flag.Parse()

	if in == nil || *in == "" {
		fmt.Fprintln(os.Stderr, "Require a package name")
		os.Exit(1)
	}

	absOut, err := filepath.Abs(*out)
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(1)
	}
	out = &absOut

	basePkg := strings.TrimPrefix(*out, path.Join(os.Getenv("GOPATH"), "src")+"/")

	ifacePkgs, implPkgs, err := genCode(*in, basePkg)
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(1)
	}

	for pkgName, pkg := range ifacePkgs {
		pkgPath := path.Join(*out, pkgName)
		err := os.MkdirAll(pkgPath, os.ModePerm)
		if err != nil {
			fmt.Fprintf(os.Stderr, err.Error())
			os.Exit(1)
		}

		pkgFile := path.Join(pkgPath, pkgName+".go")
		err = ioutil.WriteFile(pkgFile, []byte(pkg), os.ModePerm)
		if err != nil {
			fmt.Fprintf(os.Stderr, err.Error())
			os.Exit(1)
		}
	}

	for pkgName, pkg := range implPkgs {
		pkgPath := path.Join(*out, pkgName)
		err := os.MkdirAll(pkgPath, os.ModePerm)
		if err != nil {
			fmt.Fprintf(os.Stderr, err.Error())
			os.Exit(1)
		}

		pkgFile := path.Join(pkgPath, pkgName+".go")
		err = ioutil.WriteFile(pkgFile, []byte(pkg), os.ModePerm)
		if err != nil {
			fmt.Fprintf(os.Stderr, err.Error())
			os.Exit(1)
		}
	}
}

func genCode(pkgPath string, basePkg string) (map[string]string, map[string]string, error) {
	subpkgs, err := getSubpackages(pkgPath)
	if err != nil {
		return nil, nil, err
	}

	ifaceTmpl := `
// Auto generated code DO NOT EDIT
package {{.Name}}iface

{{ range $iface := .Interfaces }}
{{ $iface }}
{{ end }}
`

	implTmpl := `
// Auto generated code DO NOT EDIT
package {{.Name}}

import "{{ .ImportPath }}"
import "{{ .BasePkg }}/{{.Name}}iface"

{{ range $impl := .Implementations }}
{{ $impl }}
{{ end }}
`
	ifacePkgsMap := make(map[string]string)
	implPkgsMap := make(map[string]string)
	for subpkgName, subpkg := range subpkgs {
		ifaces, err := buildIfaces(subpkg)
		if err != nil {
			return nil, nil, err
		}

		ifacePkgBuf := new(bytes.Buffer)

		tmpl, err := template.New("iface").Parse(ifaceTmpl)
		if err != nil {
			return nil, nil, err
		}

		err = tmpl.Execute(ifacePkgBuf, struct {
			Name       string
			Interfaces []string
		}{
			Name:       subpkgName,
			Interfaces: ifaces,
		})

		ifacePkg, err := format.Source(ifacePkgBuf.Bytes())
		if err != nil {
			return nil, nil, err
		}

		impls, err := buildImpls(subpkg)
		if err != nil {
			return nil, nil, err
		}

		implPkgBuf := new(bytes.Buffer)
		tmpl, err = template.New("impl").Parse(implTmpl)
		if err != nil {
			return nil, nil, err
		}

		err = tmpl.Execute(implPkgBuf, struct {
			Name            string
			Implementations []string
			ImportPath      string
			BasePkg         string
		}{
			Name:            subpkgName,
			Implementations: impls,
			ImportPath:      subpkg.ImportPath,
			BasePkg:         basePkg,
		})

		implPkg, err := format.Source(implPkgBuf.Bytes())
		if err != nil {
			return nil, nil, err
		}

		ifacePkgsMap[subpkgName+"iface"] = string(ifacePkg)
		implPkgsMap[subpkgName] = string(implPkg)
	}

	return ifacePkgsMap, implPkgsMap, nil
}

func parsePkg(pkg string) (map[string]*ast.Package, error) {
	pkg = path.Join(os.Getenv("GOPATH"), "src", pkg)
	return parser.ParseDir(token.NewFileSet(), pkg, func(info os.FileInfo) bool {
		return !strings.Contains(info.Name(), "test")
	}, parser.ParseComments)

}

func getSubpackages(pkg string) (map[string]*Package, error) {
	subpkgs, err := parsePkg(pkg)
	if err != nil {
		return nil, err
	}

	subpkgMap := make(map[string]*Package)
	for subpkgName, subpkg := range subpkgs {
		structs, err := getStructs(subpkg)
		if err != nil {
			return nil, err
		}
		funcs, err := getFunctions(subpkg)
		if err != nil {
			return nil, err
		}
		subpkgMap[subpkgName] = &Package{
			ImportPath: pkg,
			Name:       subpkgName,
			Structs:    structs,
			Functions:  funcs,
		}
	}

	return subpkgMap, nil
}

func getFunctions(pkg *ast.Package) ([]*Function, error) {
	return []*Function{}, nil
}

func getStructs(pkg *ast.Package) ([]*Struct, error) {
	structMap := make(map[string]*Struct)

	methods, err := getMethods(pkg)
	if err != nil {
		return nil, err
	}

	fields, err := getFields(pkg)
	if err != nil {
		return nil, err
	}

	for st, stmethods := range methods {
		structMap[st] = &Struct{
			Name:    st,
			Methods: stmethods,
		}
	}

	for stName, stfields := range fields {
		st, ok := structMap[stName]
		if !ok {
			st = &Struct{
				Name:   stName,
				Fields: stfields,
			}
		} else {
			st.Fields = stfields
		}
	}

	structs := make([]*Struct, 0)
	for _, st := range structMap {
		structs = append(structs, st)
	}

	return structs, nil
}

func getMethods(pkg *ast.Package) (map[string][]*Method, error) {
	methodMap := make(map[string][]*Method)
	for fileName, astFile := range pkg.Files {
		for _, decl := range astFile.Decls {
			src, err := ioutil.ReadFile(fileName)
			if err != nil {
				continue
			}
			a, fd := maker.GetReceiverTypeName(src, decl)

			if fd != nil && ast.IsExported(fd.Name.Name) {
				methods, ok := methodMap[a]
				if !ok {
					methods = make([]*Method, 0)
				}

				// As per the docs, fd.Type.Params
				// cannot be nil but fd.Type.Results
				// can be
				params := getMethodFields(src, fd.Type.Params.List)
				results := []*Field{}
				if fd.Type.Results != nil {
					results = getMethodFields(src, fd.Type.Results.List)
				}
				methods = append(methods, &Method{
					Name:    fd.Name.Name,
					Params:  params,
					Results: results,
				})
				methodMap[a] = methods
			}
		}
	}

	return methodMap, nil
}

func getMethodFields(src []byte, astFields []*ast.Field) []*Field {
	var fields []*Field

	for _, astField := range astFields {
		field := &Field{}
		if len(astField.Names) > 0 {
			field.Name = astField.Names[0].Name
		}

		field.Type = string(src[astField.Type.Pos()-1 : astField.Type.End()-1])

		fields = append(fields, field)
	}

	return fields
}

func getFields(pkg *ast.Package) (map[string][]*Field, error) {
	fieldMap := make(map[string][]*Field)
	for fileName, astFile := range pkg.Files {
		src, err := ioutil.ReadFile(fileName)
		if err != nil {
			continue
		}

		fset := token.NewFileSet()
		_, err = parser.ParseFile(fset, fileName, nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}

		ast.Inspect(astFile, func(n ast.Node) bool {
			if st, ok := n.(*ast.StructType); ok {
				structName := getStructName(src, fset, st)
				if ast.IsExported(structName) {
					var exportedFields []*Field
					for _, astField := range st.Fields.List {
						if len(astField.Names) > 0 &&
							astField.Names[0].IsExported() {

							field := &Field{}
							field.Name = astField.Names[0].Name
							field.Type = string(src[astField.Type.Pos()-1 : astField.Type.End()-1])

							exportedFields = append(exportedFields,
								field)
						}
					}
					fieldMap[structName] = exportedFields
				}
			}
			return true
		})
	}
	return fieldMap, nil
}

func getStructName(src []byte, fset *token.FileSet, st *ast.StructType) string {
	lines := strings.Split(string(src), "\n")
	line := lines[fset.Position(st.Pos()).Line-1]
	return strings.Split(line, " ")[1]
}

func buildIfaces(pkg *Package) ([]string, error) {
	var ifaces []string

	iface := `
type {{.Name}} interface {
{{ range $field := .Fields }}
    {{ $field.Name }}() {{ $field.Type }}
{{ end }}

{{ range $method := .Methods }}
    {{ $method.Name }}({{ toList $method.Params }}) ({{ toList $method.Results }})
{{ end }}
}
`

	ifaceTmpl, err := template.New("iface").Funcs(template.FuncMap{
		"toList": func(fields []*Field) string {
			var list string
			prefix := ""
			for _, field := range fields {
				list += prefix + field.Name + " " + field.Type
				prefix = ", "
			}
			return list
		},
	}).Parse(iface)
	if err != nil {
		return []string{}, err
	}

	for _, st := range pkg.Structs {
		buf := new(bytes.Buffer)
		err := ifaceTmpl.Execute(buf, st)
		if err != nil {
			return []string{}, err
		}
		ifaces = append(ifaces, buf.String())
	}

	return ifaces, nil
}

func buildImpls(pkg *Package) ([]string, error) {
	var impls []string

	impl := `
type {{ .StructName }} struct {
    parent *{{ .PkgName }}.{{ .StructName }}
}

{{ range $field := .Fields }}
func (x *{{$.StructName}}){{ $field.Name }}() ({{ maybeAddIfacePkg $field.Type }}) {
    return x.parent.{{$field.Name}}
}
{{ end }}

{{ range $method := .Methods }}
func (x *{{$.StructName}}) {{.Name}}({{toList $method.Params}}) ({{toList $method.Results}}) {
    return x.parent.{{$method.Name}}({{argList $method.Params}})
}
{{ end }}
`

	toList := func(fields []*Field) string {
		var list string
		prefix := ""
		for _, field := range fields {
			typ := maybeAddIfacePkg(pkg, field.Type)
			list += prefix + field.Name + " " + typ
			prefix = ", "
		}
		return list
	}

	argList := func(fields []*Field) string {
		var args string
		prefix := ""
		for _, field := range fields {
			args += prefix + field.Name
			prefix = ", "
		}
		return args
	}

	implTempl, err := template.New("impl").Funcs(template.FuncMap{
		"toList":  toList,
		"argList": argList,
		"maybeAddIfacePkg": func(typ string) string {
			return maybeAddIfacePkg(pkg, typ)
		},
	}).Parse(impl)
	if err != nil {
		return []string{}, err
	}

	for _, st := range pkg.Structs {
		buf := new(bytes.Buffer)
		err := implTempl.Execute(buf, struct {
			PkgName    string
			StructName string
			Fields     []*Field
			Methods    []*Method
		}{
			PkgName:    pkg.Name,
			StructName: st.Name,
			Fields:     st.Fields,
			Methods:    st.Methods,
		})
		if err != nil {
			return []string{}, err
		}
		impls = append(impls, buf.String())
	}

	return impls, nil
}

func pkgContainsType(pkg *Package, typ string) bool {
	for _, st := range pkg.Structs {
		if st.Name == typ {
			return true
		}
	}
	return false
}

func maybeAddIfacePkg(pkg *Package, typ string) string {
	isPtr := false
	if typ[0] == '*' {
		isPtr = true
		typ = typ[1:]
	}

	if pkgContainsType(pkg, typ) {
		typ = pkg.Name + "iface." + typ
	}

	if isPtr {
		typ = "*" + typ
	}

	return typ
}
