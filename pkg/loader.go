package interfacify

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/parser"
	"go/token"
	"os/exec"
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

// packageLoader resolves and caches parsed packages across source and dependencies.
type packageLoader struct {
	// lookupRoots are the configured lookup environments.
	lookupRoots []lookupRoot
	// packageCache caches package metadata by lookup root and import path.
	packageCache map[packageCacheKey]packageInfo
	// loaded caches fully parsed source packages by import path.
	loaded map[string]*sourcePackage
}

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
	// owner is the package that declares the method signature.
	owner *sourcePackage
}

// typeRef identifies one named type in one parsed package.
type typeRef struct {
	// pkg is the package that declares the type.
	pkg *sourcePackage
	// typeName is the unqualified type name in pkg.
	typeName string
}

// embeddedPath tracks one embedded type reached through one promotion path.
type embeddedPath struct {
	// ref is the embedded type reached at the current depth.
	ref typeRef
	// visiting tracks the types already traversed on this path.
	visiting map[string]struct{}
}

// sourcePackage caches the parsed state for one source package.
type sourcePackage struct {
	// loader resolves additional packages referenced by this package.
	loader *packageLoader
	// importPath is the fully-qualified import path for the package.
	importPath string
	// dir is the package directory on disk.
	dir string
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

// newPackageLoader creates a package loader for one generation run.
func newPackageLoader(lookupRoots []lookupRoot) *packageLoader {
	return &packageLoader{
		lookupRoots:  lookupRoots,
		packageCache: map[packageCacheKey]packageInfo{},
		loaded:       map[string]*sourcePackage{},
	}
}

// load resolves and parses one package, reusing cached results.
func (loader *packageLoader) load(importPath string) (*sourcePackage, error) {
	if sourcePkg, ok := loader.loaded[importPath]; ok {
		return sourcePkg, nil
	}

	sourcePkg, err := loadSourcePackage(loader.lookupRoots, importPath, loader.packageCache)
	if err != nil {
		return nil, err
	}

	sourcePkg.loader = loader
	loader.loaded[importPath] = sourcePkg
	return sourcePkg, nil
}

// key returns a stable identifier for one named type reference.
func (ref typeRef) key() string {
	if ref.pkg == nil {
		return ref.typeName
	}

	return ref.pkg.importPath + "." + ref.typeName
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
		dir:              info.Dir,
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
								owner:   sourcePkg,
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
					owner:   sourcePkg,
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

	info, err = goListPackageInfo(lookupRoot.Path, importPath)
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

// goListPackageInfo resolves package metadata through the active Go module or workspace.
func goListPackageInfo(workDir, importPath string) (packageInfo, error) {
	cmd := exec.Command("go", "list", "-find", "-json", importPath)
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return packageInfo{}, err
	}

	var listed struct {
		Dir  string
		Name string
	}
	if err := json.Unmarshal(output, &listed); err != nil {
		return packageInfo{}, fmt.Errorf("decode go list output for %q: %w", importPath, err)
	}
	if listed.Name == "" {
		return packageInfo{}, fmt.Errorf("go list returned empty package name for %q", importPath)
	}

	return packageInfo{
		Dir:        listed.Dir,
		ImportPath: importPath,
		Name:       listed.Name,
	}, nil
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

// typeRefForExpr resolves one embedded named type expression into a package-qualified reference.
func (pkg *sourcePackage) typeRefForExpr(expr ast.Expr) (typeRef, bool, error) {
	switch expr := expr.(type) {
	case *ast.Ident:
		return typeRef{pkg: pkg, typeName: expr.Name}, true, nil
	case *ast.StarExpr:
		return pkg.typeRefForExpr(expr.X)
	case *ast.IndexExpr:
		return pkg.typeRefForExpr(expr.X)
	case *ast.IndexListExpr:
		return pkg.typeRefForExpr(expr.X)
	case *ast.SelectorExpr:
		ident, ok := expr.X.(*ast.Ident)
		if !ok {
			return typeRef{}, false, nil
		}

		importPath := pkg.imports[ident.Name]
		if importPath == "" {
			return typeRef{}, false, nil
		}
		if pkg.loader == nil {
			return typeRef{}, false, fmt.Errorf("package loader unavailable while resolving %q", importPath)
		}

		foreignPkg, err := pkg.loader.load(importPath)
		if err != nil {
			return typeRef{}, false, err
		}

		return typeRef{pkg: foreignPkg, typeName: expr.Sel.Name}, true, nil
	default:
		return typeRef{}, false, nil
	}
}

// embeddedTypeRefs resolves the embedded named types declared in one field list.
func (pkg *sourcePackage) embeddedTypeRefs(fields *ast.FieldList) ([]typeRef, error) {
	if fields == nil {
		return nil, nil
	}

	refs := make([]typeRef, 0, len(fields.List))
	for _, field := range fields.List {
		if len(field.Names) != 0 {
			continue
		}

		ref, ok, err := pkg.typeRefForExpr(field.Type)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		refs = append(refs, ref)
	}

	return refs, nil
}

// collectMethods collects declared methods and optionally embedded methods for a type.
func (pkg *sourcePackage) collectMethods(typeName string, includeEmbedded bool) ([]methodDecl, error) {
	typeSpec := pkg.typeSpecs[typeName]
	if typeSpec == nil {
		return nil, nil
	}

	ref := typeRef{pkg: pkg, typeName: typeName}
	if _, ok := typeSpec.Type.(*ast.InterfaceType); ok {
		return pkg.collectInterfaceMethods(ref, includeEmbedded)
	}

	return pkg.collectConcreteMethods(ref, includeEmbedded)
}

// collectConcreteMethods collects direct and promoted methods for one concrete type.
func (pkg *sourcePackage) collectConcreteMethods(ref typeRef, includeEmbedded bool) ([]methodDecl, error) {
	methods := make([]methodDecl, 0, len(ref.pkg.methods[ref.typeName]))
	seen := map[string]struct{}{}

	for _, method := range ref.pkg.methods[ref.typeName] {
		pkg.appendMethod(method, &methods, seen)
	}

	if !includeEmbedded {
		return methods, nil
	}

	typeSpec := ref.pkg.typeSpecs[ref.typeName]
	structType, ok := typeSpec.Type.(*ast.StructType)
	if !ok {
		return methods, nil
	}

	if err := pkg.appendPromotedMethods(ref, structType, &methods, seen); err != nil {
		return nil, err
	}

	return methods, nil
}

// collectInterfaceMethods collects direct and optionally embedded interface methods.
func (pkg *sourcePackage) collectInterfaceMethods(ref typeRef, includeEmbedded bool) ([]methodDecl, error) {
	methods := make([]methodDecl, 0, len(ref.pkg.interfaceMethods[ref.typeName]))
	seen := map[string]struct{}{}
	if err := pkg.appendInterfaceMethods(ref, includeEmbedded, &methods, seen, map[string]struct{}{}); err != nil {
		return nil, err
	}

	return methods, nil
}

// appendInterfaceMethods appends the direct and embedded methods of one interface type.
func (pkg *sourcePackage) appendInterfaceMethods(
	ref typeRef,
	includeEmbedded bool,
	methods *[]methodDecl,
	seen map[string]struct{},
	visiting map[string]struct{},
) error {
	if _, ok := visiting[ref.key()]; ok {
		return nil
	}
	visiting[ref.key()] = struct{}{}
	defer delete(visiting, ref.key())

	for _, method := range ref.pkg.interfaceMethods[ref.typeName] {
		pkg.appendMethod(method, methods, seen)
	}

	if !includeEmbedded {
		return nil
	}

	typeSpec := ref.pkg.typeSpecs[ref.typeName]
	if typeSpec == nil {
		return nil
	}

	iface, ok := typeSpec.Type.(*ast.InterfaceType)
	if !ok {
		return nil
	}

	embeddedRefs, err := ref.pkg.embeddedTypeRefs(iface.Methods)
	if err != nil {
		return err
	}
	for _, embeddedRef := range embeddedRefs {
		if err := pkg.appendInterfaceMethods(embeddedRef, true, methods, seen, visiting); err != nil {
			return err
		}
	}

	return nil
}

// appendPromotedMethods appends promoted methods using struct embedding depth rules.
func (pkg *sourcePackage) appendPromotedMethods(
	rootRef typeRef,
	structType *ast.StructType,
	methods *[]methodDecl,
	seen map[string]struct{},
) error {
	embeddedRefs, err := rootRef.pkg.embeddedTypeRefs(structType.Fields)
	if err != nil {
		return err
	}

	current := make([]embeddedPath, 0, len(embeddedRefs))
	for _, embeddedRef := range embeddedRefs {
		current = append(current, embeddedPath{
			ref:      embeddedRef,
			visiting: map[string]struct{}{rootRef.key(): {}, embeddedRef.key(): {}},
		})
	}

	ambiguous := map[string]struct{}{}
	for len(current) > 0 {
		levelMethods := map[string][]methodDecl{}
		levelOrder := make([]string, 0, len(current))
		next := make([]embeddedPath, 0, len(current))

		for _, path := range current {
			pathMethods, err := path.ref.pkg.methodsAtEmbeddingDepth(path.ref)
			if err != nil {
				return err
			}

			for _, method := range pathMethods {
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

			nextRefs, err := path.ref.pkg.nextEmbeddedTypeRefs(path.ref)
			if err != nil {
				return err
			}
			for _, embeddedRef := range nextRefs {
				if _, ok := path.visiting[embeddedRef.key()]; ok {
					continue
				}

				next = append(next, embeddedPath{
					ref:      embeddedRef,
					visiting: extendVisitedTypes(path.visiting, embeddedRef.key()),
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

	return nil
}

// methodsAtEmbeddingDepth returns the methods contributed by one embedded type at its depth.
func (pkg *sourcePackage) methodsAtEmbeddingDepth(ref typeRef) ([]methodDecl, error) {
	typeSpec := ref.pkg.typeSpecs[ref.typeName]
	if typeSpec == nil {
		return nil, nil
	}

	if _, ok := typeSpec.Type.(*ast.InterfaceType); ok {
		return pkg.collectInterfaceMethods(ref, true)
	}

	methods := make([]methodDecl, 0, len(ref.pkg.methods[ref.typeName]))
	methods = append(methods, ref.pkg.methods[ref.typeName]...)
	return methods, nil
}

// nextEmbeddedTypeRefs returns the embedded types that sit one level deeper.
func (pkg *sourcePackage) nextEmbeddedTypeRefs(ref typeRef) ([]typeRef, error) {
	typeSpec := ref.pkg.typeSpecs[ref.typeName]
	if typeSpec == nil {
		return nil, nil
	}

	structType, ok := typeSpec.Type.(*ast.StructType)
	if !ok {
		return nil, nil
	}

	return ref.pkg.embeddedTypeRefs(structType.Fields)
}

// extendVisitedTypes clones one visited set and adds the next type.
func extendVisitedTypes(visiting map[string]struct{}, key string) map[string]struct{} {
	clone := make(map[string]struct{}, len(visiting)+1)
	for name := range visiting {
		clone[name] = struct{}{}
	}
	clone[key] = struct{}{}
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

	owner := method.owner
	if owner == nil {
		owner = pkg
	}

	return owner.methodDocsByName[method.name]
}
