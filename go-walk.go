package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

const (
	SP = " "
	NL = "\n"

	UP   = +1
	DOWN = -1
)

//
// Printer is the interface to be implemented to print a program
//
type Printer interface {
	updateLevel(delta int)
	printLevel(values ...string)

	printPackage(name string)
	printImport(name, path string)
	printType(name, typedef string)
	printValue(vtype, names, typedef, values string)
}

//
// GoPrinter implement the Printer interface for Go programs
//
type GoPrinter struct {
	Printer
	level int
}

func (p *GoPrinter) updateLevel(delta int) {
	p.level += delta
}

func (p *GoPrinter) indent() string {
	return strings.Repeat("  ", p.level)
}

func (p *GoPrinter) printLevel(values ...string) {
	fmt.Print(p.indent(), strings.Join(values, " "))
}

func (p *GoPrinter) printPackage(name string) {
	p.printLevel("package", name, NL)
}

func (p *GoPrinter) printImport(name, path string) {
	p.printLevel("import", name, path, NL)
}

func (p *GoPrinter) printType(name, typedef string) {
	p.printLevel("type", name, typedef, NL)
}

func (p *GoPrinter) printValue(vtype, names, typedef, value string) {
	p.printLevel(vtype, names)
	if len(typedef) > 0 {
		fmt.Print(SP, typedef)
	}
	if len(value) > 0 {
		fmt.Print(" = ", value)
	}
	fmt.Println()
}

//
// identString return the Ident name or ""
// to use when it's ok to have an empty part (and you don't want to see '<nil>')
//
func identString(i *ast.Ident) string {
	if i == nil {
		return ""
	} else {
		return i.Name
	}
}

//
// ifTrue retruns the input value if the condition is true, an empty string otherwise
//
func ifTrue(val string, cond bool) (ret string) {
	if cond {
		ret = val
	}
	return
}

func parseExpr(expr interface{}) string {
	if expr == nil {
		return ""
	}

	switch expr := expr.(type) {

	// a name
	case *ast.Ident:
		return expr.Name

		// *thing
	case *ast.StarExpr:
		return "*" + parseExpr(expr.X)

		// [len]type
	case *ast.ArrayType:
		return fmt.Sprintf("[%s]%s", parseExpr(expr.Len), parseExpr(expr.Elt))

		// [key]value
	case *ast.MapType:
		return fmt.Sprintf("[%s]%s", parseExpr(expr.Key), parseExpr(expr.Value))

		// interface{ things }
	case *ast.InterfaceType:
		return fmt.Sprintf("interface{%s}", parseFieldList(expr.Methods, true))

		// struct{ things }
	case *ast.StructType:
		return fmt.Sprintf("struct{%s}", parseFieldList(expr.Fields, true))

		// (params...) (result)
	case *ast.FuncType:
		return fmt.Sprintf("(%s) %s", parseFieldList(expr.Params, false), parseFieldList(expr.Results, true))

		// "thing", 0, true, false, nil
	case *ast.BasicLit:
		return fmt.Sprintf("%v", expr.Value)

		// type{list}
	case *ast.CompositeLit:
		return fmt.Sprintf("%s{%s}", parseExpr(expr.Type), parseExprList(expr.Elts))

		// ...type
	case *ast.Ellipsis:
		return fmt.Sprintf("...%s", parseExpr(expr.Elt))

		// -3
	case *ast.UnaryExpr:
		return fmt.Sprintf("%s%s", expr.Op.String(), parseExpr(expr.X))

		// 3 + 2
	case *ast.BinaryExpr:
		return fmt.Sprintf("%s %s %s", parseExpr(expr.X), expr.Op.String(), parseExpr(expr.Y))

		// array[index]
	case *ast.IndexExpr:
		return fmt.Sprintf("%s[%s]", parseExpr(expr.X), parseExpr(expr.Index))

		// x[low:hi:max]
	case *ast.SliceExpr:
		if expr.Max == nil {
			return fmt.Sprintf("%s[%s:%s]", parseExpr(expr.X), parseExpr(expr.Low), parseExpr(expr.High))
		} else {
			return fmt.Sprintf("%s[%s:%s:%s]", parseExpr(expr.X), parseExpr(expr.Low), parseExpr(expr.High), parseExpr(expr.Max))
		}

		// package.member
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", parseExpr(expr.X), parseExpr(expr.Sel))

		// funcname(args)
	case *ast.CallExpr:
		return fmt.Sprintf("%s(%s%s)", parseExpr(expr.Fun), parseExprList(expr.Args), ifTrue("...", expr.Ellipsis > 0))

	default:
		return fmt.Sprintf("[[[ %#v ]]]", expr)
	}
}

func parseExprList(l []ast.Expr) string {
	exprs := []string{}
	for _, e := range l {
		exprs = append(exprs, parseExpr(e))
	}
	return strings.Join(exprs, ", ")
}

func parseFieldList(l *ast.FieldList, omitempty bool) string {
	if l != nil {
		fields := []string{}
		for _, f := range l.List {
			fields = append(fields, parseNames(f.Names)+" "+parseExpr(f.Type))
		}

		return "(" + strings.Join(fields, ", ") + ")"
	} else if omitempty {
		return ""
	} else {
		return "()"
	}
}

func parseNames(v []*ast.Ident) string {
	names := []string{}

	for _, n := range v {
		names = append(names, n.Name)
	}

	return strings.Join(names, ", ")
}

type GoWalker struct {
	p      Printer
	parent ast.Node
}

//
// Implement the Visitor interface for GoWalker
//
func (w GoWalker) Visit(node ast.Node) (ret ast.Visitor) {
	if node == nil {
		return
	}

	pparent := w.parent
	w.parent = node

	switch n := node.(type) {
	case *ast.File:
		w.p.printPackage(n.Name.String())
		for _, d := range n.Decls {
			w.Visit(d)
		}

	case *ast.ImportSpec:
		w.p.printImport(identString(n.Name), n.Path.Value)

	case *ast.TypeSpec:
		w.p.printType(n.Name.String(), parseExpr(n.Type))

	case *ast.ValueSpec:
		vtype := (pparent.(*ast.GenDecl)).Tok.String()
		w.p.printValue(vtype, parseNames(n.Names), parseExpr(n.Type), parseExprList(n.Values))

	case *ast.GenDecl:
		fmt.Println()
		for _, s := range n.Specs {
			w.Visit(s)
		}

	case *ast.FuncDecl:
		fmt.Println()
		w.p.printLevel("func", parseFieldList(n.Recv, true), n.Name.String(), parseFieldList(n.Type.Params, false), parseFieldList(n.Type.Results, true))
		w.Visit(n.Body)

	case *ast.BlockStmt:
		fmt.Println("{")
		w.p.updateLevel(UP)
		for _, i := range n.List {
			w.Visit(i)
		}
		w.p.updateLevel(DOWN)
		w.p.printLevel("}", NL)

	case *ast.IfStmt:
		w.p.printLevel("if (", parseExpr(n.Cond), ") ")
		w.Visit(n.Body)
		if n.Else != nil {
			w.p.printLevel("else ")
			w.Visit(n.Else)
		}

	case *ast.ReturnStmt:
		w.p.printLevel("return", parseExprList(n.Results), NL)

	case *ast.ExprStmt:
		w.p.printLevel(parseExpr(n.X), NL)

	case *ast.DeclStmt:
		w.Visit(n.Decl)

	case *ast.AssignStmt:
		w.p.printLevel(parseExprList(n.Lhs), n.Tok.String(), parseExprList(n.Rhs), NL)

	default:
		fmt.Printf("/* %#v */\n", n)
		ret = w
	}

	w.parent = pparent
	return
}

func main() {
	args := os.Args[1:] // skip program name
	if args[0] == "--" {
		// skip - this is to fool "go run"
		args = args[1:]
	}

	fset := token.NewFileSet() // positions are relative to fset

	f, err := parser.ParseFile(fset, args[0], nil, 0)
	if err != nil {
		fmt.Println(err)
		return
	}

	var printer GoPrinter
	var walker = GoWalker{&printer, nil}
	ast.Walk(walker, f)
}
