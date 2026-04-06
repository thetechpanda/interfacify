package encoders

import (
	"go/ast"
	"strings"
)

// CommentText returns a trimmed comment body.
func CommentText(group *ast.CommentGroup) string {
	if group == nil {
		return ""
	}

	return strings.TrimSpace(group.Text())
}

// WriteCommentLines writes text as line comments using the given indent.
func WriteCommentLines(output *strings.Builder, text, indent string) {
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		output.WriteString(indent)
		if line == "" {
			output.WriteString("//\n")
			continue
		}

		output.WriteString("// ")
		output.WriteString(line)
		output.WriteString("\n")
	}
}

// QualifyLocalTypeRefs clones expr and qualifies local named types with sourceAlias.
func QualifyLocalTypeRefs(expr ast.Expr, localTypes map[string]*ast.TypeSpec, sourceAlias string) ast.Expr {
	switch expr := expr.(type) {
	case nil:
		return nil
	case *ast.ArrayType:
		return &ast.ArrayType{
			Len: QualifyLocalTypeRefs(expr.Len, localTypes, sourceAlias),
			Elt: QualifyLocalTypeRefs(expr.Elt, localTypes, sourceAlias),
		}
	case *ast.BasicLit:
		return &ast.BasicLit{
			Kind:  expr.Kind,
			Value: expr.Value,
		}
	case *ast.BinaryExpr:
		return &ast.BinaryExpr{
			X:  QualifyLocalTypeRefs(expr.X, localTypes, sourceAlias),
			Op: expr.Op,
			Y:  QualifyLocalTypeRefs(expr.Y, localTypes, sourceAlias),
		}
	case *ast.ChanType:
		return &ast.ChanType{
			Dir:   expr.Dir,
			Value: QualifyLocalTypeRefs(expr.Value, localTypes, sourceAlias),
		}
	case *ast.Ellipsis:
		return &ast.Ellipsis{
			Elt: QualifyLocalTypeRefs(expr.Elt, localTypes, sourceAlias),
		}
	case *ast.FuncType:
		return &ast.FuncType{
			Params:  QualifyFieldListLocalTypeRefs(expr.Params, localTypes, sourceAlias),
			Results: QualifyFieldListLocalTypeRefs(expr.Results, localTypes, sourceAlias),
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
			X:     QualifyLocalTypeRefs(expr.X, localTypes, sourceAlias),
			Index: QualifyLocalTypeRefs(expr.Index, localTypes, sourceAlias),
		}
	case *ast.IndexListExpr:
		indices := make([]ast.Expr, 0, len(expr.Indices))
		for _, index := range expr.Indices {
			indices = append(indices, QualifyLocalTypeRefs(index, localTypes, sourceAlias))
		}

		return &ast.IndexListExpr{
			X:       QualifyLocalTypeRefs(expr.X, localTypes, sourceAlias),
			Indices: indices,
		}
	case *ast.InterfaceType:
		return &ast.InterfaceType{
			Methods:    QualifyFieldListLocalTypeRefs(expr.Methods, localTypes, sourceAlias),
			Incomplete: expr.Incomplete,
		}
	case *ast.MapType:
		return &ast.MapType{
			Key:   QualifyLocalTypeRefs(expr.Key, localTypes, sourceAlias),
			Value: QualifyLocalTypeRefs(expr.Value, localTypes, sourceAlias),
		}
	case *ast.ParenExpr:
		return &ast.ParenExpr{X: QualifyLocalTypeRefs(expr.X, localTypes, sourceAlias)}
	case *ast.SelectorExpr:
		ident, ok := expr.X.(*ast.Ident)
		if ok {
			return &ast.SelectorExpr{
				X:   ast.NewIdent(ident.Name),
				Sel: ast.NewIdent(expr.Sel.Name),
			}
		}

		return &ast.SelectorExpr{
			X:   QualifyLocalTypeRefs(expr.X, localTypes, sourceAlias),
			Sel: ast.NewIdent(expr.Sel.Name),
		}
	case *ast.StarExpr:
		return &ast.StarExpr{X: QualifyLocalTypeRefs(expr.X, localTypes, sourceAlias)}
	case *ast.StructType:
		return &ast.StructType{
			Fields:     QualifyFieldListLocalTypeRefs(expr.Fields, localTypes, sourceAlias),
			Incomplete: expr.Incomplete,
		}
	case *ast.UnaryExpr:
		return &ast.UnaryExpr{
			Op: expr.Op,
			X:  QualifyLocalTypeRefs(expr.X, localTypes, sourceAlias),
		}
	default:
		return expr
	}
}

// QualifyFieldListLocalTypeRefs clones list and qualifies local named types.
func QualifyFieldListLocalTypeRefs(list *ast.FieldList, localTypes map[string]*ast.TypeSpec, sourceAlias string) *ast.FieldList {
	if list == nil {
		return nil
	}

	fields := make([]*ast.Field, 0, len(list.List))
	for _, field := range list.List {
		fields = append(fields, QualifyFieldLocalTypeRefs(field, localTypes, sourceAlias))
	}

	return &ast.FieldList{List: fields}
}

// QualifyFieldLocalTypeRefs clones field and qualifies local named types in its type.
func QualifyFieldLocalTypeRefs(field *ast.Field, localTypes map[string]*ast.TypeSpec, sourceAlias string) *ast.Field {
	if field == nil {
		return nil
	}

	names := make([]*ast.Ident, 0, len(field.Names))
	for _, name := range field.Names {
		names = append(names, ast.NewIdent(name.Name))
	}

	return &ast.Field{
		Names: names,
		Type:  QualifyLocalTypeRefs(field.Type, localTypes, sourceAlias),
		Tag:   CloneBasicLit(field.Tag),
	}
}

// CloneBasicLit clones one basic literal.
func CloneBasicLit(lit *ast.BasicLit) *ast.BasicLit {
	if lit == nil {
		return nil
	}

	return &ast.BasicLit{
		Kind:  lit.Kind,
		Value: lit.Value,
	}
}

// ExprUsesLocalTypes reports whether an expression refers to local named types.
func ExprUsesLocalTypes(expr ast.Expr, localTypes map[string]*ast.TypeSpec) bool {
	switch expr := expr.(type) {
	case nil:
		return false
	case *ast.Ident:
		_, ok := localTypes[expr.Name]
		return ok
	case *ast.ArrayType:
		return ExprUsesLocalTypes(expr.Len, localTypes) || ExprUsesLocalTypes(expr.Elt, localTypes)
	case *ast.BinaryExpr:
		return ExprUsesLocalTypes(expr.X, localTypes) || ExprUsesLocalTypes(expr.Y, localTypes)
	case *ast.ChanType:
		return ExprUsesLocalTypes(expr.Value, localTypes)
	case *ast.Ellipsis:
		return ExprUsesLocalTypes(expr.Elt, localTypes)
	case *ast.FuncType:
		return FieldListUsesLocalTypes(expr.Params, localTypes) || FieldListUsesLocalTypes(expr.Results, localTypes)
	case *ast.IndexExpr:
		return ExprUsesLocalTypes(expr.X, localTypes) || ExprUsesLocalTypes(expr.Index, localTypes)
	case *ast.IndexListExpr:
		if ExprUsesLocalTypes(expr.X, localTypes) {
			return true
		}
		for _, index := range expr.Indices {
			if ExprUsesLocalTypes(index, localTypes) {
				return true
			}
		}
		return false
	case *ast.InterfaceType:
		return FieldListUsesLocalTypes(expr.Methods, localTypes)
	case *ast.MapType:
		return ExprUsesLocalTypes(expr.Key, localTypes) || ExprUsesLocalTypes(expr.Value, localTypes)
	case *ast.ParenExpr:
		return ExprUsesLocalTypes(expr.X, localTypes)
	case *ast.SelectorExpr:
		return false
	case *ast.StarExpr:
		return ExprUsesLocalTypes(expr.X, localTypes)
	case *ast.StructType:
		return FieldListUsesLocalTypes(expr.Fields, localTypes)
	case *ast.UnaryExpr:
		return ExprUsesLocalTypes(expr.X, localTypes)
	default:
		return false
	}
}

// FieldListUsesLocalTypes reports whether any field type refers to a local named type.
func FieldListUsesLocalTypes(list *ast.FieldList, localTypes map[string]*ast.TypeSpec) bool {
	if list == nil {
		return false
	}

	for _, field := range list.List {
		if ExprUsesLocalTypes(field.Type, localTypes) {
			return true
		}
	}

	return false
}

// FirstUnexportedLocalType returns the first unexported local named type used by expr.
func FirstUnexportedLocalType(expr ast.Expr, localTypes map[string]*ast.TypeSpec) (string, bool) {
	switch expr := expr.(type) {
	case nil:
		return "", false
	case *ast.Ident:
		if _, ok := localTypes[expr.Name]; ok && !ast.IsExported(expr.Name) {
			return expr.Name, true
		}
		return "", false
	case *ast.ArrayType:
		if typeName, ok := FirstUnexportedLocalType(expr.Len, localTypes); ok {
			return typeName, true
		}
		return FirstUnexportedLocalType(expr.Elt, localTypes)
	case *ast.BinaryExpr:
		if typeName, ok := FirstUnexportedLocalType(expr.X, localTypes); ok {
			return typeName, true
		}
		return FirstUnexportedLocalType(expr.Y, localTypes)
	case *ast.ChanType:
		return FirstUnexportedLocalType(expr.Value, localTypes)
	case *ast.Ellipsis:
		return FirstUnexportedLocalType(expr.Elt, localTypes)
	case *ast.FuncType:
		if typeName, ok := FirstUnexportedLocalTypeInFieldList(expr.Params, localTypes); ok {
			return typeName, true
		}
		return FirstUnexportedLocalTypeInFieldList(expr.Results, localTypes)
	case *ast.IndexExpr:
		if typeName, ok := FirstUnexportedLocalType(expr.X, localTypes); ok {
			return typeName, true
		}
		return FirstUnexportedLocalType(expr.Index, localTypes)
	case *ast.IndexListExpr:
		if typeName, ok := FirstUnexportedLocalType(expr.X, localTypes); ok {
			return typeName, true
		}
		for _, index := range expr.Indices {
			if typeName, ok := FirstUnexportedLocalType(index, localTypes); ok {
				return typeName, true
			}
		}
		return "", false
	case *ast.InterfaceType:
		return FirstUnexportedLocalTypeInFieldList(expr.Methods, localTypes)
	case *ast.MapType:
		if typeName, ok := FirstUnexportedLocalType(expr.Key, localTypes); ok {
			return typeName, true
		}
		return FirstUnexportedLocalType(expr.Value, localTypes)
	case *ast.ParenExpr:
		return FirstUnexportedLocalType(expr.X, localTypes)
	case *ast.SelectorExpr:
		return "", false
	case *ast.StarExpr:
		return FirstUnexportedLocalType(expr.X, localTypes)
	case *ast.StructType:
		return FirstUnexportedLocalTypeInFieldList(expr.Fields, localTypes)
	case *ast.UnaryExpr:
		return FirstUnexportedLocalType(expr.X, localTypes)
	default:
		return "", false
	}
}

// FirstUnexportedLocalTypeInFieldList returns the first unexported local type used in list.
func FirstUnexportedLocalTypeInFieldList(list *ast.FieldList, localTypes map[string]*ast.TypeSpec) (string, bool) {
	if list == nil {
		return "", false
	}

	for _, field := range list.List {
		if typeName, ok := FirstUnexportedLocalType(field.Type, localTypes); ok {
			return typeName, true
		}
	}

	return "", false
}
