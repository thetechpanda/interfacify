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
	// usedImports collects imports referenced by rendered signatures.
	usedImports map[string]string
	// packageAliases stores chosen aliases for packages qualified during rendering.
	packageAliases map[string]string
}

// renderMethod renders one interface method signature.
func (renderer *signatureRenderer) renderMethod(method methodDecl) (string, error) {
	owner := method.owner
	if owner == nil {
		owner = renderer.pkg
	}

	params, _, err := renderer.renderFieldList(method.params, owner)
	if err != nil {
		return "", err
	}

	results, resultCount, err := renderer.renderFieldList(method.results, owner)
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
func (renderer *signatureRenderer) renderTypeParams(list *ast.FieldList, owner *sourcePackage) (string, error) {
	if list == nil || len(list.List) == 0 {
		return "", nil
	}

	parts := make([]string, 0, len(list.List))
	for _, field := range list.List {
		constraint, err := renderer.renderExpr(field.Type, owner)
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
func (renderer *signatureRenderer) renderFieldList(list *ast.FieldList, owner *sourcePackage) (string, int, error) {
	if list == nil || len(list.List) == 0 {
		return "", 0, nil
	}

	parts := make([]string, 0, len(list.List))
	count := 0
	for _, field := range list.List {
		fieldType, err := renderer.renderExpr(field.Type, owner)
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
func (renderer *signatureRenderer) renderExpr(expr ast.Expr, owner *sourcePackage) (string, error) {
	qualifyLocalTypes := !outputMatchesSourcePackage(renderer.outputDir, renderer.outputPkg, owner)
	if qualifyLocalTypes {
		if typeName, ok := encoders.FirstUnexportedLocalType(expr, owner.typeSpecs); ok {
			switch {
			case owner.importPath != renderer.pkg.importPath:
				return "", fmt.Errorf(
					"method signature from imported package %q uses unexported local type %q",
					owner.importPath,
					typeName,
				)
			case renderer.outputPkg != owner.name:
				return "", fmt.Errorf(
					"output package %q differs from source package %q for a method signature that uses unexported local type %q",
					renderer.outputPkg,
					owner.name,
					typeName,
				)
			default:
				return "", fmt.Errorf(
					"output file in %q is outside source package directory %q for a method signature that uses unexported local type %q",
					renderer.outputDir,
					owner.dir,
					typeName,
				)
			}
		}
	}

	if err := renderer.collectImports(expr, owner); err != nil {
		return "", err
	}

	renderExpr := expr
	if qualifyLocalTypes && encoders.ExprUsesLocalTypes(expr, owner.typeSpecs) {
		renderExpr = encoders.QualifyLocalTypeRefs(expr, owner.typeSpecs, renderer.ensurePackageImportAlias(owner))
	}

	var output strings.Builder
	if err := printer.Fprint(&output, owner.fset, renderExpr); err != nil {
		return "", err
	}

	return output.String(), nil
}

// ensurePackageImportAlias returns the alias used for one qualified package import.
func (renderer *signatureRenderer) ensurePackageImportAlias(pkg *sourcePackage) string {
	if alias, ok := renderer.packageAliases[pkg.importPath]; ok {
		return alias
	}

	preferred := pkg.name
	if preferred == "" {
		preferred = decoders.DefaultImportName(pkg.importPath)
	}

	alias := preferred
	if existing, ok := renderer.usedImports[alias]; ok && existing != pkg.importPath {
		for i := 1; ; i++ {
			candidate := fmt.Sprintf("%s%d", preferred, i)
			if existing, ok := renderer.usedImports[candidate]; !ok || existing == pkg.importPath {
				alias = candidate
				break
			}
		}
	}

	renderer.usedImports[alias] = pkg.importPath
	renderer.packageAliases[pkg.importPath] = alias
	return alias
}

// outputMatchesSourcePackage reports whether the generated file belongs to the source package.
func outputMatchesSourcePackage(outputDir, outputPkg string, pkg *sourcePackage) bool {
	return outputPkg == pkg.name && outputDir == pkg.dir
}

// collectImports records package imports used by a rendered expression.

func (renderer *signatureRenderer) collectImports(expr ast.Expr, owner *sourcePackage) error {
	var collectErr error
	ast.Inspect(expr, func(node ast.Node) bool {
		if collectErr != nil {
			return false
		}

		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		ident, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}

		importPath := owner.imports[ident.Name]
		if importPath != "" {
			if existing, ok := renderer.usedImports[ident.Name]; ok && existing != importPath {
				collectErr = fmt.Errorf("import name %q is used by both %q and %q", ident.Name, existing, importPath)
				return false
			}

			renderer.usedImports[ident.Name] = importPath
		}

		return true
	})

	return collectErr
}
