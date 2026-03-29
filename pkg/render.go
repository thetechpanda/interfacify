package interfacify

import (
	"fmt"
	"go/ast"
	"go/printer"
	"strings"
)

// signatureRenderer formats method signatures and tracks required imports.
type signatureRenderer struct {
	// outputPkg is the package name used for the generated file.
	outputPkg string
	// pkg is the source package being rendered.
	pkg *sourcePackage
	// usedImports collects imports referenced by rendered signatures.
	usedImports map[string]string
}

// renderMethod renders one interface method signature.
func (renderer *signatureRenderer) renderMethod(method methodDecl) (string, error) {
	params, _, err := renderer.renderFieldList(method.params)
	if err != nil {
		return "", err
	}

	results, resultCount, err := renderer.renderFieldList(method.results)
	if err != nil {
		return "", err
	}

	var output strings.Builder
	output.WriteString(method.name)
	output.WriteString("(")
	output.WriteString(params)
	output.WriteString(")")

	switch resultCount {
	case 0:
		return output.String(), nil
	case 1:
		output.WriteString(" ")
		output.WriteString(results)
	default:
		output.WriteString(" (")
		output.WriteString(results)
		output.WriteString(")")
	}

	return output.String(), nil
}

// renderTypeParams renders a generic type parameter list for one type declaration.
func (renderer *signatureRenderer) renderTypeParams(list *ast.FieldList) (string, error) {
	if list == nil || len(list.List) == 0 {
		return "", nil
	}

	parts := make([]string, 0, len(list.List))
	for _, field := range list.List {
		constraint, err := renderer.renderExpr(field.Type)
		if err != nil {
			return "", err
		}

		names := make([]string, 0, len(field.Names))
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
		if len(names) == 0 {
			parts = append(parts, constraint)
			continue
		}

		part := strings.Join(names, ", ")
		if constraint != "" {
			part += " " + constraint
		}

		parts = append(parts, part)
	}

	return "[" + strings.Join(parts, ", ") + "]", nil
}

// renderFieldList renders an AST field list into a comma-separated type list.
func (renderer *signatureRenderer) renderFieldList(list *ast.FieldList) (string, int, error) {
	if list == nil || len(list.List) == 0 {
		return "", 0, nil
	}

	parts := make([]string, 0, len(list.List))
	count := 0
	for _, field := range list.List {
		fieldType, err := renderer.renderExpr(field.Type)
		if err != nil {
			return "", 0, err
		}

		repeats := len(field.Names)
		if repeats == 0 {
			repeats = 1
		}

		for range repeats {
			parts = append(parts, fieldType)
			count++
		}
	}

	return strings.Join(parts, ", "), count, nil
}

// renderExpr renders one type expression and validates local type usage.
func (renderer *signatureRenderer) renderExpr(expr ast.Expr) (string, error) {
	if renderer.outputPkg != renderer.pkg.name && exprUsesLocalTypes(expr, renderer.pkg.typeSpecs) {
		return "", fmt.Errorf(
			"output package %q differs from source package %q for a method signature that uses local package types",
			renderer.outputPkg,
			renderer.pkg.name,
		)
	}

	renderer.collectImports(expr)

	var output strings.Builder
	if err := printer.Fprint(&output, renderer.pkg.fset, expr); err != nil {
		return "", err
	}

	return output.String(), nil
}

// collectImports records package imports used by a rendered expression.
func (renderer *signatureRenderer) collectImports(expr ast.Expr) {
	ast.Inspect(expr, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		ident, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}

		importPath := renderer.pkg.imports[ident.Name]
		if importPath != "" {
			renderer.usedImports[ident.Name] = importPath
		}

		return true
	})
}

// exprUsesLocalTypes reports whether an expression refers to local named types.
func exprUsesLocalTypes(expr ast.Expr, localTypes map[string]*ast.TypeSpec) bool {
	switch expr := expr.(type) {
	case nil:
		return false
	case *ast.Ident:
		_, ok := localTypes[expr.Name]
		return ok
	case *ast.ArrayType:
		return exprUsesLocalTypes(expr.Len, localTypes) || exprUsesLocalTypes(expr.Elt, localTypes)
	case *ast.BinaryExpr:
		return exprUsesLocalTypes(expr.X, localTypes) || exprUsesLocalTypes(expr.Y, localTypes)
	case *ast.ChanType:
		return exprUsesLocalTypes(expr.Value, localTypes)
	case *ast.Ellipsis:
		return exprUsesLocalTypes(expr.Elt, localTypes)
	case *ast.FuncType:
		return fieldListUsesLocalTypes(expr.Params, localTypes) || fieldListUsesLocalTypes(expr.Results, localTypes)
	case *ast.IndexExpr:
		return exprUsesLocalTypes(expr.X, localTypes) || exprUsesLocalTypes(expr.Index, localTypes)
	case *ast.IndexListExpr:
		if exprUsesLocalTypes(expr.X, localTypes) {
			return true
		}
		for _, index := range expr.Indices {
			if exprUsesLocalTypes(index, localTypes) {
				return true
			}
		}
		return false
	case *ast.InterfaceType:
		return fieldListUsesLocalTypes(expr.Methods, localTypes)
	case *ast.MapType:
		return exprUsesLocalTypes(expr.Key, localTypes) || exprUsesLocalTypes(expr.Value, localTypes)
	case *ast.ParenExpr:
		return exprUsesLocalTypes(expr.X, localTypes)
	case *ast.SelectorExpr:
		return exprUsesLocalTypes(expr.X, localTypes)
	case *ast.StarExpr:
		return exprUsesLocalTypes(expr.X, localTypes)
	case *ast.StructType:
		return fieldListUsesLocalTypes(expr.Fields, localTypes)
	case *ast.UnaryExpr:
		return exprUsesLocalTypes(expr.X, localTypes)
	default:
		return false
	}
}

// fieldListUsesLocalTypes reports whether any field type refers to a local named type.
func fieldListUsesLocalTypes(list *ast.FieldList, localTypes map[string]*ast.TypeSpec) bool {
	if list == nil {
		return false
	}

	for _, field := range list.List {
		if exprUsesLocalTypes(field.Type, localTypes) {
			return true
		}
	}

	return false
}
