package lint

import (
	"go/ast"
	"go/token"
)

type (
	// funcDeclChecker visits every top-level function declaration.
	//
	// See also: baseFuncDeclChecker, wrapFuncDeclChecker.
	funcDeclChecker interface {
		CheckFuncDecl(*ast.FuncDecl)
	}

	// exprChecker visits every expression inside AST file.
	//
	// See also: baseExprChecker, wrapExprChecker.
	exprChecker interface {
		CheckExpr(ast.Expr)
	}

	// localExprChecker visits every expression inside function body.
	//
	// PerFuncInit is called for every function visited.
	// If returned false, function is skipped.
	//
	// See also: baseLocalExprChecker, wrapLocalExprChecker.
	localExprChecker interface {
		PerFuncInit(*ast.FuncDecl) bool
		CheckLocalExpr(ast.Expr)
	}

	// stmtListChecker visits every statement list inside function body.
	// This includes block statement bodies as well as implicit blocks
	// introduced by case clauses and alike.
	//
	// See also: baseStmtListChecker, wrapStmtListChecker.
	stmtListChecker interface {
		CheckStmtList([]ast.Stmt)
	}

	// stmtChecker visits every statement inside function body.
	//
	// PerFuncInit is called for every function visited.
	// If returned false, function is skipped.
	//
	// See also: baseStmtChecker, wrapStmtChecker.
	stmtChecker interface {
		PerFuncInit(*ast.FuncDecl) bool
		CheckStmt(ast.Stmt)
	}

	// localNameChecker visits every name definition inside function.
	// Next elements are considered as name definitions:
	//	- Function parameters (input, output, receiver)
	//	- Every LHS of ":=" assignment
	//	- Every local var/const declaration.
	//
	// See also: baseLocalNameChecker, wrapLocalNameChecker.
	localNameChecker interface {
		CheckLocalName(*ast.Ident)
	}

	// typeExpeChecker visits every type describing expression.
	// It also traverses struct types and interface types to run
	// checker over their fields/method signatures.
	//
	// See also: baseTypeExprChecker, wrapTypeExprChecker.
	typeExprChecker interface {
		CheckTypeExpr(ast.Expr)
	}
)

type baseFuncDeclChecker struct {
	ctx *context
}

func wrapFuncDeclChecker(c funcDeclChecker) func(*ast.File) {
	return func(f *ast.File) {
		for _, decl := range f.Decls {
			if decl, ok := decl.(*ast.FuncDecl); ok {
				c.CheckFuncDecl(decl)
			}
		}
	}
}

type baseExprChecker struct {
	ctx *context
}

func wrapExprChecker(c exprChecker) func(*ast.File) {
	return func(f *ast.File) {
		ast.Inspect(f, func(x ast.Node) bool {
			if expr, ok := x.(ast.Expr); ok {
				c.CheckExpr(expr)
			}
			return true
		})
	}
}

type baseLocalExprChecker struct {
	ctx *context
}

func wrapLocalExprChecker(c localExprChecker) func(*ast.File) {
	return func(f *ast.File) {
		for _, decl := range f.Decls {
			decl, ok := decl.(*ast.FuncDecl)
			if !ok || !c.PerFuncInit(decl) {
				continue
			}
			ast.Inspect(decl.Body, func(x ast.Node) bool {
				if expr, ok := x.(ast.Expr); ok {
					c.CheckLocalExpr(expr)
				}
				return true
			})
		}
	}
}

func (c baseLocalExprChecker) PerFuncInit(fn *ast.FuncDecl) bool {
	return fn.Body != nil
}

type baseStmtListChecker struct {
	ctx *context
}

func wrapStmtListChecker(c stmtListChecker) func(*ast.File) {
	return func(f *ast.File) {
		for _, decl := range f.Decls {
			decl, ok := decl.(*ast.FuncDecl)
			if !ok || decl.Body == nil {
				continue
			}
			ast.Inspect(decl.Body, func(x ast.Node) bool {
				switch x := x.(type) {
				case *ast.BlockStmt:
					c.CheckStmtList(x.List)
				case *ast.CaseClause:
					c.CheckStmtList(x.Body)
				case *ast.CommClause:
					c.CheckStmtList(x.Body)
				}
				return true
			})
		}
	}
}

type baseStmtChecker struct {
	ctx *context
}

func (c baseStmtChecker) PerFuncInit(fn *ast.FuncDecl) bool {
	return fn.Body != nil
}

func wrapStmtChecker(c stmtChecker) func(*ast.File) {
	return func(f *ast.File) {
		for _, decl := range f.Decls {
			// Only functions can contain statements.
			decl, ok := decl.(*ast.FuncDecl)
			if !ok || !c.PerFuncInit(decl) {
				continue
			}
			ast.Inspect(decl.Body, func(x ast.Node) bool {
				if stmt, ok := x.(ast.Stmt); ok {
					c.CheckStmt(stmt)
				}
				return true
			})
		}
	}
}

type baseLocalNameChecker struct {
	ctx *context
}

func wrapLocalNameChecker(c localNameChecker) func(*ast.File) {
	return func(f *ast.File) {
		for _, decl := range f.Decls {
			decl, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			// First, function params.
			ast.Inspect(decl.Type, func(x ast.Node) bool {
				if id, ok := x.(*ast.Ident); ok {
					c.CheckLocalName(id)
				}
				return true
			})
			if decl.Recv != nil {
				c.CheckLocalName(decl.Recv.List[0].Names[0])
			}
			if decl.Body == nil { // Skip external functions
				return
			}
			// Now every assignment and var/const decl.
			ast.Inspect(decl.Body, func(x ast.Node) bool {
				switch x := x.(type) {
				case *ast.AssignStmt:
					// "=" can never introduce new names.
					if x.Tok == token.ASSIGN {
						return false
					}
					// Can't be precise without type info here,
					// some identifiers passed to CheckLocalName
					// are not defs, but rather re-declarations.
					for _, lhs := range x.Lhs {
						if lhs, ok := lhs.(*ast.Ident); ok {
							c.CheckLocalName(lhs)
						}
					}
					return false
				case *ast.GenDecl:
					for _, spec := range x.Specs {
						spec, ok := spec.(*ast.ValueSpec)
						if !ok { // Ignore type specs
							return false
						}
						for _, id := range spec.Names {
							c.CheckLocalName(id)
						}
					}
					return false
				}
				return true
			})
		}
	}
}

type baseTypeExprChecker struct {
	ctx *context
}

func wrapTypeExprChecker(c typeExprChecker) func(*ast.File) {
	var checkExpr func(x ast.Expr)

	checkStructType := func(x *ast.StructType) {
		for _, field := range x.Fields.List {
			checkExpr(field.Type)
		}
	}
	checkFieldList := func(xs []*ast.Field) {
		for _, x := range xs {
			checkExpr(x.Type)
		}
	}
	checkFuncType := func(x *ast.FuncType) {
		checkFieldList(x.Params.List)
		if x.Results != nil {
			checkFieldList(x.Results.List)
		}
	}

	checkExpr = func(x ast.Expr) {
		switch x := x.(type) {
		case *ast.CompositeLit:
			checkExpr(x.Type)
		case *ast.StructType:
			checkStructType(x)
		case *ast.InterfaceType:
			checkFieldList(x.Methods.List)
		case *ast.FuncType:
			checkFuncType(x)
		case *ast.ArrayType:
			c.CheckTypeExpr(x)
		case *ast.FuncLit:
			checkExpr(x.Type)
		default:
			c.CheckTypeExpr(x)
		}
	}

	checkGenDecl := func(x *ast.GenDecl) {
		for _, spec := range x.Specs {
			switch spec := spec.(type) {
			case *ast.ValueSpec:
				if spec.Type != nil {
					checkExpr(spec.Type)
				}
				for _, expr := range spec.Values {
					checkExpr(expr)
				}
			case *ast.TypeSpec:
				checkExpr(spec.Type)
			default:
				// Do nothing for *ast.ImportSpec.
			}
		}
	}

	return func(f *ast.File) {
		for _, decl := range f.Decls {
			if decl, ok := decl.(*ast.GenDecl); ok {
				checkGenDecl(decl)
				continue
			}

			// Must be a func decl.
			decl := decl.(*ast.FuncDecl)
			if decl.Recv != nil {
				checkExpr(decl.Recv.List[0].Type)
			}
			checkFuncType(decl.Type)
			if decl.Body == nil {
				continue
			}
			for _, stmt := range decl.Body.List {
				// TODO: need to look inside expressions to detect
				// calls like make(T, ...), where T is an expression
				// we want to check.
				switch stmt := stmt.(type) {
				case *ast.DeclStmt:
					// Function-local declaration of var/const/type.
					checkGenDecl(stmt.Decl.(*ast.GenDecl))
				}
			}
		}
	}
}
