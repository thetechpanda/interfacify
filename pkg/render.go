package interfacify

import (
	"fmt"
	"go/ast"
	"go/printer"
	"strings"

	decoders "github.com/thetechpanda/interfacify/pkg/decoders"
	encoders "github.com/thetechpanda/interfacify/pkg/encoders"
)

// signatureRenderer formats method signatures and tracks required imports.
type signatureRenderer struct {
	// outputPkg is the package name used for the generated file.
	outputPkg string
	// outputDir is the directory where the generated file will be written.
	outputDir string
	// pkg is the source package being rendered.
	pkg *sourcePackage
	// qualifyLocalTypes reports whether source-local types must be package-qualified.
	qualifyLocalTypes bool
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
	if renderer.qualifyLocalTypes {
		if typeName, ok := encoders.FirstUnexportedLocalType(expr, renderer.pkg.typeSpecs); ok {
			if renderer.outputPkg != renderer.pkg.name {
				return "", fmt.Errorf(
					"output package %q differs from source package %q for a method signature that uses unexported local type %q",
					renderer.outputPkg,
					renderer.pkg.name,
					typeName,
				)
			}

			return "", fmt.Errorf(
				"output file in %q is outside source package directory %q for a method signature that uses unexported local type %q",
				renderer.outputDir,
				renderer.pkg.dir,
				typeName,
			)
		}
	}

	renderer.collectImports(expr)

	renderExpr := expr
	if renderer.qualifyLocalTypes && encoders.ExprUsesLocalTypes(expr, renderer.pkg.typeSpecs) {
		renderExpr = encoders.QualifyLocalTypeRefs(expr, renderer.pkg.typeSpecs, renderer.ensureSourceImportAlias())
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
		preferred = decoders.DefaultImportName(renderer.pkg.importPath)
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

// outputMatchesSourcePackage reports whether the generated file belongs to the source package.
func outputMatchesSourcePackage(outputDir, outputPkg string, pkg *sourcePackage) bool {
	return outputPkg == pkg.name && outputDir == pkg.dir
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
