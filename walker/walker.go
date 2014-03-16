package walkngo

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/raff/walkngo/printer"
)

//
// GoWalker is the context for the AST visitor
//
type GoWalker struct {
	p      printer.Printer
	parent ast.Node
	buffer bytes.Buffer
	flush  bool
}

func NewWalker(p printer.Printer) *GoWalker {
	w := GoWalker{p: p, flush: true}
	p.SetWriter(&w.buffer)
	return &w
}

func (w *GoWalker) WalkFile(filename string) error {
	fset := token.NewFileSet() // positions are relative to fset

	f, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		return err
	}

	ast.Walk(w, f)
	return nil
}

//
// Implement the Visitor interface for GoWalker
//
func (w *GoWalker) Visit(node ast.Node) (ret ast.Visitor) {
	if node == nil {
		return
	}

	pparent := w.parent
	w.parent = node

	switch n := node.(type) {
	case *ast.File:
		w.p.PrintPackage(n.Name.String())
		for _, d := range n.Decls {
			w.Visit(d)
		}

	case *ast.ImportSpec:
		w.p.PrintImport(identString(n.Name), n.Path.Value)

	case *ast.TypeSpec:
		w.p.PrintType(n.Name.String(), w.parseExpr(n.Type))

	case *ast.ValueSpec:
		vtype := (pparent.(*ast.GenDecl)).Tok.String()
		w.p.PrintValue(vtype, w.parseNames(n.Names), w.parseExpr(n.Type), w.parseExprList(n.Values))

	case *ast.GenDecl:
		for _, s := range n.Specs {
			w.Visit(s)
		}

	case *ast.FuncDecl:
		w.p.PrintFunc(w.parseFieldList(n.Recv, ", "),
			n.Name.String(),
			w.parseFieldList(n.Type.Params, ", "),
			w.parseFieldList(n.Type.Results, ", "))
		w.Visit(n.Body)
		w.p.Print("\n")

	case *ast.BlockStmt:
		w.p.PrintLevel("{\n")
		w.p.UpdateLevel(printer.UP)
		for _, i := range n.List {
			w.Visit(i)
		}
		w.p.UpdateLevel(printer.DOWN)
		w.p.PrintLevel("}")

	case *ast.IfStmt:
		w.p.PrintIf(w.BufferVisit(n.Init), w.parseExpr(n.Cond))
		w.Visit(n.Body)
		if n.Else != nil {
			w.p.SameLine()
			w.p.PrintElse()
			w.Visit(n.Else)
		}
		w.p.Print("\n")

	case *ast.ForStmt:
		w.p.PrintFor(w.BufferVisit(n.Init), w.parseExpr(n.Cond), w.BufferVisit(n.Post))
		w.Visit(n.Body)
		w.p.Print("\n")

	case *ast.SwitchStmt:
		w.p.PrintSwitch(w.BufferVisit(n.Init), w.parseExpr(n.Tag))
		w.Visit(n.Body)
		w.p.Print("\n")

	case *ast.TypeSwitchStmt:
		w.p.PrintSwitch(w.BufferVisit(n.Init), w.BufferVisit(n.Assign))
		w.Visit(n.Body)
		w.p.Print("\n")

	case *ast.CaseClause:
		if len(n.List) > 0 {
			w.p.PrintLevel("case", w.parseExprList(n.List), ":", "\n")
		} else {
			w.p.PrintLevel("default:", "\n")
		}
		w.p.UpdateLevel(printer.UP)
		for _, i := range n.Body {
			w.Visit(i)
		}
		w.p.UpdateLevel(printer.DOWN)

	case *ast.RangeStmt:
		w.p.PrintLevel("for", w.parseExpr(n.Key), ",", w.parseExpr(n.Value), ":= range", w.parseExpr(n.X))
		w.Visit(n.Body)
		w.p.Print("\n")

	case *ast.BranchStmt:
		w.p.PrintStmt(n.Tok.String(), identString(n.Label))

	case *ast.DeferStmt:
		w.p.PrintStmt("defer", w.parseExpr(n.Call))

	case *ast.GoStmt:
		w.p.PrintStmt("go", w.parseExpr(n.Call))

	case *ast.ReturnStmt:
		w.p.PrintStmt("return", w.parseExprList(n.Results))

	case *ast.ExprStmt:
		w.p.PrintLevel(w.parseExpr(n.X), "\n")

	case *ast.DeclStmt:
		w.Visit(n.Decl)

	case *ast.AssignStmt:
		w.p.PrintLevel(w.parseExprList(n.Lhs), n.Tok.String(), w.parseExprList(n.Rhs), "\n")

	case *ast.IncDecStmt:
		w.p.PrintLevel(w.parseExpr(n.X)+n.Tok.String(), " ")

	case *ast.EmptyStmt:
		w.p.PrintEmpty()

	default:
		w.p.Print(fmt.Sprintf("/* Node: %#v */\n", n))
		ret = w
	}

	w.Flush()

	w.parent = pparent
	return
}

func (w *GoWalker) Flush() {
	if w.flush && w.buffer.Len() > 0 {
		fmt.Print(w.buffer.String())
		w.buffer.Reset()
	}
}

func (w *GoWalker) BufferVisit(node ast.Node) (ret string) {
	w.Flush()

	prev := w.flush
	w.flush = false
	w.Visit(node)
	w.flush = prev

	ret = strings.TrimSpace(w.buffer.String())

	if w.flush {
		w.buffer.Reset()
	}

	return
}

func (w *GoWalker) parseExpr(expr interface{}) string {
	if expr == nil {
		return ""
	}

	switch expr := expr.(type) {

	// a name
	case *ast.Ident:
		return expr.Name

		// *thing
	case *ast.StarExpr:
		return "*" + w.parseExpr(expr.X)

		// [len]type
	case *ast.ArrayType:
		return fmt.Sprintf("[%s]%s", w.parseExpr(expr.Len), w.parseExpr(expr.Elt))

		// [key]value
	case *ast.MapType:
		return fmt.Sprintf("[%s]%s", w.parseExpr(expr.Key), w.parseExpr(expr.Value))

		// interface{ things }
	case *ast.InterfaceType:
		return fmt.Sprintf("interface{%s}", w.parseFieldList(expr.Methods, "; "))

		// struct{ things }
	case *ast.StructType:
		return fmt.Sprintf("struct{%s}", w.parseFieldList(expr.Fields, "; "))

		// <-chan type
	case *ast.ChanType:
		ctype := "chan"
		if expr.Dir == ast.SEND {
			ctype = "chan<-"
		} else if expr.Dir == ast.RECV {
			ctype = "<-chan"
		}
		return fmt.Sprintf("%s %s", ctype, w.parseExpr(expr.Value))
		break

		// (params...) (result)
	case *ast.FuncType:
		return fmt.Sprintf("(%s) %s", w.parseFieldList(expr.Params, ", "), wrapIf(w.parseFieldList(expr.Results, ", ")))

		// "thing", 0, true, false, nil
	case *ast.BasicLit:
		return fmt.Sprintf("%v", expr.Value)

		// type{list}
	case *ast.CompositeLit:
		return fmt.Sprintf("%s{%s}", w.parseExpr(expr.Type), w.parseExprList(expr.Elts))

		// ...type
	case *ast.Ellipsis:
		return fmt.Sprintf("...%s", w.parseExpr(expr.Elt))

		// -3
	case *ast.UnaryExpr:
		return fmt.Sprintf("%s%s", expr.Op.String(), w.parseExpr(expr.X))

		// 3 + 2
	case *ast.BinaryExpr:
		return fmt.Sprintf("%s %s %s", w.parseExpr(expr.X), expr.Op.String(), w.parseExpr(expr.Y))

		// array[index]
	case *ast.IndexExpr:
		return fmt.Sprintf("%s[%s]", w.parseExpr(expr.X), w.parseExpr(expr.Index))

		// key: value
	case *ast.KeyValueExpr:
		return fmt.Sprintf("%s: %s", w.parseExpr(expr.Key), w.parseExpr(expr.Value))

		// x[low:hi:max]
	case *ast.SliceExpr:
		if expr.Max == nil {
			return fmt.Sprintf("%s[%s:%s]", w.parseExpr(expr.X), w.parseExpr(expr.Low), w.parseExpr(expr.High))
		} else {
			return fmt.Sprintf("%s[%s:%s:%s]", w.parseExpr(expr.X), w.parseExpr(expr.Low), w.parseExpr(expr.High), w.parseExpr(expr.Max))
		}

		// package.member
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", w.parseExpr(expr.X), w.parseExpr(expr.Sel))

		// funcname(args)
	case *ast.CallExpr:
		return fmt.Sprintf("%s(%s%s)", w.parseExpr(expr.Fun), w.parseExprList(expr.Args), ifTrue("...", expr.Ellipsis > 0))

		// name.(type)
	case *ast.TypeAssertExpr:
		return fmt.Sprintf("%s.(%s)", w.parseExpr(expr.X), w.exprOr(expr.Type, "type"))

		// (expr)
	case *ast.ParenExpr:
		return fmt.Sprintf("(%s)", w.parseExpr(expr.X))

		// func(params) (ret) { body }
	case *ast.FuncLit:
		return fmt.Sprintf("func%s %s", w.parseExpr(expr.Type), w.BufferVisit(expr.Body))
	}

	return fmt.Sprintf("/* Expr: %#v */", expr)
}

func (w *GoWalker) parseExprList(l []ast.Expr) string {
	exprs := []string{}
	for _, e := range l {
		exprs = append(exprs, w.parseExpr(e))
	}
	return strings.Join(exprs, ", ")
}

func (w *GoWalker) parseFieldList(l *ast.FieldList, sep string) string {
	if l != nil {
		fields := []string{}
		for _, f := range l.List {
			field := w.parseNames(f.Names)
			if len(field) > 0 {
				field += " " + w.parseExpr(f.Type)
			} else {
				field = w.parseExpr(f.Type)
			}
			fields = append(fields, field)
		}

		return strings.Join(fields, sep)
	} else {
		return ""
	}
}

func (w *GoWalker) parseNames(v []*ast.Ident) string {
	names := []string{}

	for _, n := range v {
		names = append(names, n.Name)
	}

	return strings.Join(names, ", ")
}

func (w *GoWalker) exprOr(expr ast.Expr, v string) string {
	if expr != nil {
		return w.parseExpr(expr)
	} else {
		return v
	}
}

//
// identString return the Ident name or ""
// to use when it's ok to have an empty part (and you don't want to see '<nil>')
//
func identString(i *ast.Ident) (ret string) {
	if i != nil {
		ret = i.Name
	}
	return
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

func wrapIf(val string) (ret string) {
	if len(val) > 0 {
		ret = fmt.Sprintf("(%s)", val)
	}
	return
}
