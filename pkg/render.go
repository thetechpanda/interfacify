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
	// sourceImportAlias is the alias used when qualifying local source-package types.
	sourceImportAlias string
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

// renderExpr renders one type expression and qualifies local types when needed.
func (renderer *signatureRenderer) renderExpr(expr ast.Expr) (string, error) {
	if renderer.outputPkg != renderer.pkg.name {
		if typeName, ok := firstUnexportedLocalType(expr, renderer.pkg.typeSpecs); ok {
			return "", fmt.Errorf(
				"output package %q differs from source package %q for a method signature that uses unexported local type %q",
				renderer.outputPkg,
				renderer.pkg.name,
				typeName,
			)
		}
	}

	renderer.collectImports(expr)

	renderExpr := expr
	if renderer.outputPkg != renderer.pkg.name && exprUsesLocalTypes(expr, renderer.pkg.typeSpecs) {
		renderExpr = qualifyLocalTypeRefs(expr, renderer.pkg.typeSpecs, renderer.ensureSourceImportAlias())
	}

	var output strings.Builder
	if err := printer.Fprint(&output, renderer.pkg.fset, renderExpr); err != nil {
		return "", err
	}

	return output.String(), nil
}

// ensureSourceImportAlias returns the alias used for the source package import.
func (renderer *signatureRenderer) ensureSourceImportAlias() string {
	if renderer.sourceImportAlias != "" {
		return renderer.sourceImportAlias
	}

	preferred := renderer.pkg.name
	if preferred == "" {
		preferred = defaultImportName(renderer.pkg.importPath)
	}

	alias := preferred
	if existing, ok := renderer.usedImports[alias]; ok && existing != renderer.pkg.importPath {
		for i := 1; ; i++ {
			candidate := fmt.Sprintf("%s%d", preferred, i)
			if existing, ok := renderer.usedImports[candidate]; !ok || existing == renderer.pkg.importPath {
				alias = candidate
				break
			}
		}
	}

	renderer.usedImports[alias] = renderer.pkg.importPath
	renderer.sourceImportAlias = alias
	return alias
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

// qualifyLocalTypeRefs clones expr and qualifies local named types with sourceAlias.
func qualifyLocalTypeRefs(expr ast.Expr, localTypes map[string]*ast.TypeSpec, sourceAlias string) ast.Expr {
	switch expr := expr.(type) {
	case nil:
		return nil
	case *ast.ArrayType:
		return &ast.ArrayType{
			Len: qualifyLocalTypeRefs(expr.Len, localTypes, sourceAlias),
			Elt: qualifyLocalTypeRefs(expr.Elt, localTypes, sourceAlias),
		}
	case *ast.BasicLit:
		return &ast.BasicLit{
			Kind:  expr.Kind,
			Value: expr.Value,
		}
	case *ast.BinaryExpr:
		return &ast.BinaryExpr{
			X:  qualifyLocalTypeRefs(expr.X, localTypes, sourceAlias),
			Op: expr.Op,
			Y:  qualifyLocalTypeRefs(expr.Y, localTypes, sourceAlias),
		}
	case *ast.ChanType:
		return &ast.ChanType{
			Dir:   expr.Dir,
			Value: qualifyLocalTypeRefs(expr.Value, localTypes, sourceAlias),
		}
	case *ast.Ellipsis:
		return &ast.Ellipsis{
			Elt: qualifyLocalTypeRefs(expr.Elt, localTypes, sourceAlias),
		}
	case *ast.FuncType:
		return &ast.FuncType{
			Params:  qualifyFieldListLocalTypeRefs(expr.Params, localTypes, sourceAlias),
			Results: qualifyFieldListLocalTypeRefs(expr.Results, localTypes, sourceAlias),
		}
	case *ast.Ident:
		if _, ok := localTypes[expr.Name]; ok {
			return &ast.SelectorExpr{
				X:   ast.NewIdent(sourceAlias),
				Sel: ast.NewIdent(expr.Name),
			}
		}

		return ast.NewIdent(expr.Name)
	case *ast.IndexExpr:
		return &ast.IndexExpr{
			X:     qualifyLocalTypeRefs(expr.X, localTypes, sourceAlias),
			Index: qualifyLocalTypeRefs(expr.Index, localTypes, sourceAlias),
		}
	case *ast.IndexListExpr:
		indices := make([]ast.Expr, 0, len(expr.Indices))
		for _, index := range expr.Indices {
			indices = append(indices, qualifyLocalTypeRefs(index, localTypes, sourceAlias))
		}

		return &ast.IndexListExpr{
			X:       qualifyLocalTypeRefs(expr.X, localTypes, sourceAlias),
			Indices: indices,
		}
	case *ast.InterfaceType:
		return &ast.InterfaceType{
			Methods:    qualifyFieldListLocalTypeRefs(expr.Methods, localTypes, sourceAlias),
			Incomplete: expr.Incomplete,
		}
	case *ast.MapType:
		return &ast.MapType{
			Key:   qualifyLocalTypeRefs(expr.Key, localTypes, sourceAlias),
			Value: qualifyLocalTypeRefs(expr.Value, localTypes, sourceAlias),
		}
	case *ast.ParenExpr:
		return &ast.ParenExpr{X: qualifyLocalTypeRefs(expr.X, localTypes, sourceAlias)}
	case *ast.SelectorExpr:
		ident, ok := expr.X.(*ast.Ident)
		if ok {
			return &ast.SelectorExpr{
				X:   ast.NewIdent(ident.Name),
				Sel: ast.NewIdent(expr.Sel.Name),
			}
		}

		return &ast.SelectorExpr{
			X:   qualifyLocalTypeRefs(expr.X, localTypes, sourceAlias),
			Sel: ast.NewIdent(expr.Sel.Name),
		}
	case *ast.StarExpr:
		return &ast.StarExpr{X: qualifyLocalTypeRefs(expr.X, localTypes, sourceAlias)}
	case *ast.StructType:
		return &ast.StructType{
			Fields:     qualifyFieldListLocalTypeRefs(expr.Fields, localTypes, sourceAlias),
			Incomplete: expr.Incomplete,
		}
	case *ast.UnaryExpr:
		return &ast.UnaryExpr{
			Op: expr.Op,
			X:  qualifyLocalTypeRefs(expr.X, localTypes, sourceAlias),
		}
	default:
		return expr
	}
}

// qualifyFieldListLocalTypeRefs clones list and qualifies local named types.
func qualifyFieldListLocalTypeRefs(list *ast.FieldList, localTypes map[string]*ast.TypeSpec, sourceAlias string) *ast.FieldList {
	if list == nil {
		return nil
	}

	fields := make([]*ast.Field, 0, len(list.List))
	for _, field := range list.List {
		fields = append(fields, qualifyFieldLocalTypeRefs(field, localTypes, sourceAlias))
	}

	return &ast.FieldList{List: fields}
}

// qualifyFieldLocalTypeRefs clones field and qualifies local named types in its type.
func qualifyFieldLocalTypeRefs(field *ast.Field, localTypes map[string]*ast.TypeSpec, sourceAlias string) *ast.Field {
	if field == nil {
		return nil
	}

	names := make([]*ast.Ident, 0, len(field.Names))
	for _, name := range field.Names {
		names = append(names, ast.NewIdent(name.Name))
	}

	return &ast.Field{
		Names: names,
		Type:  qualifyLocalTypeRefs(field.Type, localTypes, sourceAlias),
		Tag:   cloneBasicLit(field.Tag),
	}
}

// cloneBasicLit clones one basic literal.
func cloneBasicLit(lit *ast.BasicLit) *ast.BasicLit {
	if lit == nil {
		return nil
	}

	return &ast.BasicLit{
		Kind:  lit.Kind,
		Value: lit.Value,
	}
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
		return false
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

// firstUnexportedLocalType returns the first unexported local named type used by expr.
func firstUnexportedLocalType(expr ast.Expr, localTypes map[string]*ast.TypeSpec) (string, bool) {
	switch expr := expr.(type) {
	case nil:
		return "", false
	case *ast.Ident:
		if _, ok := localTypes[expr.Name]; ok && !ast.IsExported(expr.Name) {
			return expr.Name, true
		}
		return "", false
	case *ast.ArrayType:
		if typeName, ok := firstUnexportedLocalType(expr.Len, localTypes); ok {
			return typeName, true
		}
		return firstUnexportedLocalType(expr.Elt, localTypes)
	case *ast.BinaryExpr:
		if typeName, ok := firstUnexportedLocalType(expr.X, localTypes); ok {
			return typeName, true
		}
		return firstUnexportedLocalType(expr.Y, localTypes)
	case *ast.ChanType:
		return firstUnexportedLocalType(expr.Value, localTypes)
	case *ast.Ellipsis:
		return firstUnexportedLocalType(expr.Elt, localTypes)
	case *ast.FuncType:
		if typeName, ok := firstUnexportedLocalTypeInFieldList(expr.Params, localTypes); ok {
			return typeName, true
		}
		return firstUnexportedLocalTypeInFieldList(expr.Results, localTypes)
	case *ast.IndexExpr:
		if typeName, ok := firstUnexportedLocalType(expr.X, localTypes); ok {
			return typeName, true
		}
		return firstUnexportedLocalType(expr.Index, localTypes)
	case *ast.IndexListExpr:
		if typeName, ok := firstUnexportedLocalType(expr.X, localTypes); ok {
			return typeName, true
		}
		for _, index := range expr.Indices {
			if typeName, ok := firstUnexportedLocalType(index, localTypes); ok {
				return typeName, true
			}
		}
		return "", false
	case *ast.InterfaceType:
		return firstUnexportedLocalTypeInFieldList(expr.Methods, localTypes)
	case *ast.MapType:
		if typeName, ok := firstUnexportedLocalType(expr.Key, localTypes); ok {
			return typeName, true
		}
		return firstUnexportedLocalType(expr.Value, localTypes)
	case *ast.ParenExpr:
		return firstUnexportedLocalType(expr.X, localTypes)
	case *ast.SelectorExpr:
		return "", false
	case *ast.StarExpr:
		return firstUnexportedLocalType(expr.X, localTypes)
	case *ast.StructType:
		return firstUnexportedLocalTypeInFieldList(expr.Fields, localTypes)
	case *ast.UnaryExpr:
		return firstUnexportedLocalType(expr.X, localTypes)
	default:
		return "", false
	}
}

// firstUnexportedLocalTypeInFieldList returns the first unexported local type used in list.
func firstUnexportedLocalTypeInFieldList(list *ast.FieldList, localTypes map[string]*ast.TypeSpec) (string, bool) {
	if list == nil {
		return "", false
	}

	for _, field := range list.List {
		if typeName, ok := firstUnexportedLocalType(field.Type, localTypes); ok {
			return typeName, true
		}
	}

	return "", false
}
