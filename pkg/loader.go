package interfacify

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"

	decoders "github.com/thetechpanda/interfacify/pkg/decoders"
	encoders "github.com/thetechpanda/interfacify/pkg/encoders"
)

// packageCacheKey identifies one resolved package for a lookup path.
type packageCacheKey struct {
	lookupPath string
	importPath string
}

// lookupRoot stores one configured lookup path and the modules it exposes.
type lookupRoot = decoders.LookupRoot

// moduleRoot stores one resolved module root available from a lookup path.
type moduleRoot = decoders.ModuleRoot

// packageInfo stores the source metadata needed to parse one package.
type packageInfo struct {
	// Dir is the package directory on disk.
	Dir string
	// ImportPath is the package import path.
	ImportPath string
	// Name is the declared Go package name.
	Name string
	// GoFiles lists the package source files relative to Dir.
	GoFiles []string
}

// methodDecl stores the pieces needed to render one interface method.
type methodDecl struct {
	// name is the method name.
	name string
	// doc is the method documentation, if present.
	doc string
	// params contains the parsed parameter list.
	params *ast.FieldList
	// results contains the parsed result list.
	results *ast.FieldList
}

// embeddedPath tracks one embedded type reached through one promotion path.
type embeddedPath struct {
	// typeName is the embedded local type name reached at the current depth.
	typeName string
	// visiting tracks the local types already traversed on this path.
	visiting map[string]struct{}
}

// sourcePackage caches the parsed state for one source package.
type sourcePackage struct {
	// importPath is the fully-qualified import path for the package.
	importPath string
	// name is the declared Go package name.
	name string
	// fset tracks token positions for the parsed files.
	fset *token.FileSet
	// typeSpecs indexes named type declarations by name.
	typeSpecs map[string]*ast.TypeSpec
	// typeDocs indexes type documentation by type name.
	typeDocs map[string]string
	// methods indexes concrete receiver methods by receiver type.
	methods map[string][]methodDecl
	// interfaceMethods indexes interface-declared methods by interface name.
	interfaceMethods map[string][]methodDecl
	// methodDocsByName indexes method docs by method name for fallback lookup.
	methodDocsByName map[string]string
	// imports maps in-package import names to their import paths.
	imports map[string]string
}

// loadSourcePackage loads and indexes one source package.
func loadSourcePackage(lookupRoots []lookupRoot, importPath string, packageCache map[packageCacheKey]packageInfo) (*sourcePackage, error) {
	info, resolvedRoot, err := resolvePackage(lookupRoots, importPath, packageCache)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	files := make([]*ast.File, 0, len(info.GoFiles))
	docFiles := make(map[string]*ast.File, len(info.GoFiles))
	for _, fileName := range info.GoFiles {
		fullPath := filepath.Join(info.Dir, fileName)
		file, err := parser.ParseFile(fset, fullPath, nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}

		files = append(files, file)
		docFiles[fullPath] = file
	}

	sourcePkg := &sourcePackage{
		importPath:       info.ImportPath,
		name:             info.Name,
		fset:             fset,
		typeSpecs:        map[string]*ast.TypeSpec{},
		typeDocs:         map[string]string{},
		methods:          map[string][]methodDecl{},
		interfaceMethods: map[string][]methodDecl{},
		methodDocsByName: map[string]string{},
		imports:          map[string]string{},
	}

	docPkg := doc.New(
		&ast.Package{
			Name:  info.Name,
			Files: docFiles,
		},
		info.ImportPath,
		doc.AllDecls,
	)
	for _, typeDoc := range docPkg.Types {
		docText := strings.TrimSpace(typeDoc.Doc)
		if docText != "" {
			sourcePkg.typeDocs[typeDoc.Name] = docText
		}

		for _, methodDoc := range typeDoc.Methods {
			docText := strings.TrimSpace(methodDoc.Doc)
			if docText != "" && sourcePkg.methodDocsByName[methodDoc.Name] == "" {
				sourcePkg.methodDocsByName[methodDoc.Name] = docText
			}
		}
	}

	for _, file := range files {
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return nil, err
			}

			importName, err := resolveImportName(resolvedRoot, importPath, spec, packageCache)
			if err != nil {
				return nil, err
			}
			if importName == "" || importName == "." || importName == "_" {
				continue
			}

			sourcePkg.imports[importName] = importPath
		}

		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.GenDecl:
				if decl.Tok != token.TYPE {
					continue
				}

				for _, spec := range decl.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}

					sourcePkg.typeSpecs[typeSpec.Name.Name] = typeSpec

					iface, ok := typeSpec.Type.(*ast.InterfaceType)
					if !ok {
						continue
					}

					for _, field := range iface.Methods.List {
						if len(field.Names) == 0 {
							continue
						}

						funcType, ok := field.Type.(*ast.FuncType)
						if !ok {
							continue
						}

						methodDoc := encoders.CommentText(field.Doc)
						if methodDoc == "" {
							methodDoc = encoders.CommentText(field.Comment)
						}

						for _, name := range field.Names {
							if !ast.IsExported(name.Name) {
								continue
							}

							method := methodDecl{
								name:    name.Name,
								doc:     methodDoc,
								params:  funcType.Params,
								results: funcType.Results,
							}
							sourcePkg.interfaceMethods[typeSpec.Name.Name] = append(sourcePkg.interfaceMethods[typeSpec.Name.Name], method)
							if methodDoc != "" && sourcePkg.methodDocsByName[name.Name] == "" {
								sourcePkg.methodDocsByName[name.Name] = methodDoc
							}
						}
					}
				}

			case *ast.FuncDecl:
				if decl.Recv == nil || len(decl.Recv.List) == 0 || !ast.IsExported(decl.Name.Name) {
					continue
				}

				recvName := receiverName(decl.Recv.List[0].Type)
				if recvName == "" {
					continue
				}

				methodDoc := encoders.CommentText(decl.Doc)
				sourcePkg.methods[recvName] = append(sourcePkg.methods[recvName], methodDecl{
					name:    decl.Name.Name,
					doc:     methodDoc,
					params:  decl.Type.Params,
					results: decl.Type.Results,
				})
				if methodDoc != "" && sourcePkg.methodDocsByName[decl.Name.Name] == "" {
					sourcePkg.methodDocsByName[decl.Name.Name] = methodDoc
				}
			}
		}
	}

	return sourcePkg, nil
}

// resolvePackage finds and caches one package from the configured lookup roots.
func resolvePackage(lookupRoots []lookupRoot, importPath string, cache map[packageCacheKey]packageInfo) (packageInfo, lookupRoot, error) {
	var failures []string
	for _, lookupRoot := range lookupRoots {
		info, err := resolvePackageInRoot(lookupRoot, importPath, cache)
		if err == nil {
			return info, lookupRoot, nil
		}

		failures = append(failures, fmt.Sprintf("%s: %v", lookupRoot.Path, err))
	}

	return packageInfo{}, lookupRoot{}, fmt.Errorf(
		"package %q not found in configured lookup paths: %s",
		importPath,
		strings.Join(failures, "; "),
	)
}

// resolvePackageInRoot finds and caches one package from a single lookup root.
func resolvePackageInRoot(lookupRoot lookupRoot, importPath string, cache map[packageCacheKey]packageInfo) (packageInfo, error) {
	key := packageCacheKey{
		lookupPath: lookupRoot.Path,
		importPath: importPath,
	}
	if info, ok := cache[key]; ok {
		return info, nil
	}

	module, relPath, ok := lookupRoot.BestModuleForImport(importPath)
	if !ok {
		return packageInfo{}, fmt.Errorf("no module matches import path %q", importPath)
	}

	dir := module.Dir
	if relPath != "" {
		dir = filepath.Join(dir, filepath.FromSlash(relPath))
	}

	info, err := loadPackageInfo(dir, importPath)
	if err != nil {
		return packageInfo{}, err
	}

	cache[key] = info
	return info, nil
}

// loadPackageInfo reads package metadata directly from a source directory.
func loadPackageInfo(dir, importPath string) (packageInfo, error) {
	buildPkg, err := build.Default.ImportDir(dir, 0)
	if err != nil {
		return packageInfo{}, fmt.Errorf("load package %q from %q: %w", importPath, dir, err)
	}

	goFiles := append([]string{}, buildPkg.GoFiles...)
	goFiles = append(goFiles, buildPkg.CgoFiles...)
	if len(goFiles) == 0 {
		return packageInfo{}, fmt.Errorf("package %q at %q has no buildable Go files", importPath, dir)
	}

	return packageInfo{
		Dir:        dir,
		ImportPath: importPath,
		Name:       buildPkg.Name,
		GoFiles:    goFiles,
	}, nil
}

// resolveImportName resolves the identifier used for one import in source code.
func resolveImportName(lookupRoot lookupRoot, importPath string, spec *ast.ImportSpec, packageCache map[packageCacheKey]packageInfo) (string, error) {
	if spec.Name != nil {
		return spec.Name.Name, nil
	}
	if importPath == "C" {
		return "C", nil
	}

	info, err := resolvePackageInRoot(lookupRoot, importPath, packageCache)
	if err == nil {
		return info.Name, nil
	}

	stdlibPkg, buildErr := build.Default.Import(importPath, "", build.FindOnly)
	if buildErr == nil {
		info, infoErr := loadPackageInfo(stdlibPkg.Dir, importPath)
		if infoErr == nil {
			return info.Name, nil
		}
	}

	return decoders.DefaultImportName(importPath), nil
}

// receiverName returns the local receiver type name for a method receiver expression.
func receiverName(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.Ident:
		return expr.Name
	case *ast.StarExpr:
		return receiverName(expr.X)
	case *ast.IndexExpr:
		return receiverName(expr.X)
	case *ast.IndexListExpr:
		return receiverName(expr.X)
	default:
		return ""
	}
}

// localTypeName returns the local named type referenced by an embedded field.
func localTypeName(expr ast.Expr) (string, bool) {
	switch expr := expr.(type) {
	case *ast.Ident:
		return expr.Name, true
	case *ast.StarExpr:
		return localTypeName(expr.X)
	case *ast.IndexExpr:
		return localTypeName(expr.X)
	case *ast.IndexListExpr:
		return localTypeName(expr.X)
	default:
		return "", false
	}
}

// collectMethods collects declared methods and optionally embedded methods for a type.
func (pkg *sourcePackage) collectMethods(typeName string, includeEmbedded bool) []methodDecl {
	typeSpec := pkg.typeSpecs[typeName]
	if typeSpec == nil {
		return nil
	}

	if _, ok := typeSpec.Type.(*ast.InterfaceType); ok {
		return pkg.collectInterfaceMethods(typeName, includeEmbedded)
	}

	return pkg.collectConcreteMethods(typeName, includeEmbedded)
}

// collectConcreteMethods collects direct and promoted methods for one concrete type.
func (pkg *sourcePackage) collectConcreteMethods(typeName string, includeEmbedded bool) []methodDecl {
	methods := make([]methodDecl, 0, len(pkg.methods[typeName]))
	seen := map[string]struct{}{}

	for _, method := range pkg.methods[typeName] {
		pkg.appendMethod(method, &methods, seen)
	}

	if !includeEmbedded {
		return methods
	}

	typeSpec := pkg.typeSpecs[typeName]
	structType, ok := typeSpec.Type.(*ast.StructType)
	if !ok {
		return methods
	}

	pkg.appendPromotedMethods(typeName, structType, &methods, seen)
	return methods
}

// collectInterfaceMethods collects direct and optionally embedded interface methods.
func (pkg *sourcePackage) collectInterfaceMethods(typeName string, includeEmbedded bool) []methodDecl {
	methods := make([]methodDecl, 0, len(pkg.interfaceMethods[typeName]))
	seen := map[string]struct{}{}
	pkg.appendInterfaceMethods(typeName, includeEmbedded, &methods, seen, map[string]struct{}{})
	return methods
}

// appendInterfaceMethods appends the direct and embedded methods of one interface type.
func (pkg *sourcePackage) appendInterfaceMethods(
	typeName string,
	includeEmbedded bool,
	methods *[]methodDecl,
	seen map[string]struct{},
	visiting map[string]struct{},
) {
	if _, ok := visiting[typeName]; ok {
		return
	}
	visiting[typeName] = struct{}{}
	defer delete(visiting, typeName)

	for _, method := range pkg.interfaceMethods[typeName] {
		pkg.appendMethod(method, methods, seen)
	}

	if !includeEmbedded {
		return
	}

	typeSpec := pkg.typeSpecs[typeName]
	if typeSpec == nil {
		return
	}

	iface, ok := typeSpec.Type.(*ast.InterfaceType)
	if !ok {
		return
	}

	for _, embeddedType := range localEmbeddedTypes(iface.Methods) {
		pkg.appendInterfaceMethods(embeddedType, true, methods, seen, visiting)
	}
}

// appendPromotedMethods appends promoted methods using struct embedding depth rules.
func (pkg *sourcePackage) appendPromotedMethods(
	rootTypeName string,
	structType *ast.StructType,
	methods *[]methodDecl,
	seen map[string]struct{},
) {
	current := make([]embeddedPath, 0, len(structType.Fields.List))
	for _, embeddedType := range localEmbeddedTypes(structType.Fields) {
		current = append(current, embeddedPath{
			typeName: embeddedType,
			visiting: map[string]struct{}{rootTypeName: {}, embeddedType: {}},
		})
	}

	ambiguous := map[string]struct{}{}
	for len(current) > 0 {
		levelMethods := map[string][]methodDecl{}
		levelOrder := make([]string, 0, len(current))
		next := make([]embeddedPath, 0, len(current))

		for _, path := range current {
			for _, method := range pkg.methodsAtEmbeddingDepth(path.typeName) {
				if _, ok := seen[method.name]; ok {
					continue
				}
				if _, ok := ambiguous[method.name]; ok {
					continue
				}
				if _, ok := levelMethods[method.name]; !ok {
					levelOrder = append(levelOrder, method.name)
				}
				levelMethods[method.name] = append(levelMethods[method.name], method)
			}

			for _, embeddedType := range pkg.nextEmbeddedLocalTypes(path.typeName) {
				if _, ok := path.visiting[embeddedType]; ok {
					continue
				}

				next = append(next, embeddedPath{
					typeName: embeddedType,
					visiting: extendVisitedTypes(path.visiting, embeddedType),
				})
			}
		}

		for _, name := range levelOrder {
			candidates := levelMethods[name]
			if len(candidates) != 1 {
				ambiguous[name] = struct{}{}
				continue
			}

			pkg.appendMethod(candidates[0], methods, seen)
		}

		current = next
	}
}

// methodsAtEmbeddingDepth returns the methods contributed by one embedded type at its depth.
func (pkg *sourcePackage) methodsAtEmbeddingDepth(typeName string) []methodDecl {
	typeSpec := pkg.typeSpecs[typeName]
	if typeSpec == nil {
		return nil
	}

	if _, ok := typeSpec.Type.(*ast.InterfaceType); ok {
		return pkg.collectInterfaceMethods(typeName, true)
	}

	methods := make([]methodDecl, 0, len(pkg.methods[typeName]))
	methods = append(methods, pkg.methods[typeName]...)
	return methods
}

// nextEmbeddedLocalTypes returns the local embedded types that sit one level deeper.
func (pkg *sourcePackage) nextEmbeddedLocalTypes(typeName string) []string {
	typeSpec := pkg.typeSpecs[typeName]
	if typeSpec == nil {
		return nil
	}

	structType, ok := typeSpec.Type.(*ast.StructType)
	if !ok {
		return nil
	}

	return localEmbeddedTypes(structType.Fields)
}

// localEmbeddedTypes returns the local embedded type names from one field list.
func localEmbeddedTypes(fields *ast.FieldList) []string {
	if fields == nil {
		return nil
	}

	types := make([]string, 0, len(fields.List))
	for _, field := range fields.List {
		if len(field.Names) != 0 {
			continue
		}

		embeddedType, ok := localTypeName(field.Type)
		if !ok {
			continue
		}

		types = append(types, embeddedType)
	}

	return types
}

// extendVisitedTypes clones one visited set and adds the next type.
func extendVisitedTypes(visiting map[string]struct{}, typeName string) map[string]struct{} {
	clone := make(map[string]struct{}, len(visiting)+1)
	for name := range visiting {
		clone[name] = struct{}{}
	}
	clone[typeName] = struct{}{}
	return clone
}

// appendMethod appends a method once based on its name.
func (pkg *sourcePackage) appendMethod(method methodDecl, methods *[]methodDecl, seen map[string]struct{}) {
	if _, ok := seen[method.name]; ok {
		return
	}
	seen[method.name] = struct{}{}
	*methods = append(*methods, method)
}

// docForMethod returns the most specific documentation available for a method.
func (pkg *sourcePackage) docForMethod(method methodDecl) string {
	if method.doc != "" {
		return method.doc
	}

	return pkg.methodDocsByName[method.name]
}
