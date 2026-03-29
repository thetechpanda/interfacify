package interfacify

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// packageCacheKey identifies one resolved package for a lookup path.
type packageCacheKey struct {
	lookupPath string
	importPath string
}

// lookupRoot stores one configured lookup path and the modules it exposes.
type lookupRoot struct {
	path    string
	modules []moduleRoot
}

// moduleRoot stores one resolved module root available from a lookup path.
type moduleRoot struct {
	dir        string
	modulePath string
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

						methodDoc := commentText(field.Doc)
						if methodDoc == "" {
							methodDoc = commentText(field.Comment)
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

				methodDoc := commentText(decl.Doc)
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

// buildLookupRoots resolves configured lookup paths into module roots.
func buildLookupRoots(lookupPaths []string) ([]lookupRoot, error) {
	roots := make([]lookupRoot, 0, len(lookupPaths))
	for _, lookupPath := range lookupPaths {
		root, err := buildLookupRoot(lookupPath)
		if err != nil {
			return nil, err
		}

		roots = append(roots, root)
	}

	return roots, nil
}

// buildLookupRoot resolves one lookup path into a module or workspace root.
func buildLookupRoot(lookupPath string) (lookupRoot, error) {
	rootDir, rootKind, err := findLookupEnvironment(lookupPath)
	if err != nil {
		return lookupRoot{}, err
	}

	var modules []moduleRoot
	switch rootKind {
	case "work":
		modules, err = loadWorkspaceModules(rootDir)
	case "mod":
		modules, err = loadModuleRoots(rootDir)
	default:
		err = fmt.Errorf("unsupported lookup root type %q", rootKind)
	}
	if err != nil {
		return lookupRoot{}, err
	}

	return lookupRoot{
		path:    lookupPath,
		modules: modules,
	}, nil
}

// findLookupEnvironment finds the effective module or workspace root for one path.
func findLookupEnvironment(startDir string) (string, string, error) {
	dir := startDir
	var moduleDir string
	for {
		workFile := filepath.Join(dir, "go.work")
		if _, err := os.Stat(workFile); err == nil {
			return dir, "work", nil
		} else if !errorsIsNotExist(err) {
			return "", "", fmt.Errorf("stat %q: %w", workFile, err)
		}

		modFile := filepath.Join(dir, "go.mod")
		if moduleDir == "" {
			if _, err := os.Stat(modFile); err == nil {
				moduleDir = dir
			} else if !errorsIsNotExist(err) {
				return "", "", fmt.Errorf("stat %q: %w", modFile, err)
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	if moduleDir != "" {
		return moduleDir, "mod", nil
	}

	return "", "", fmt.Errorf("no go.mod or go.work found for lookup path %q", startDir)
}

// loadWorkspaceModules loads all module roots referenced by a go.work file.
func loadWorkspaceModules(workDir string) ([]moduleRoot, error) {
	useDirs, err := parseGoWorkUseDirs(workDir)
	if err != nil {
		return nil, err
	}

	modules := make([]moduleRoot, 0, len(useDirs))
	seen := make(map[string]struct{}, len(useDirs))
	for _, useDir := range useDirs {
		moduleDir, err := resolveDir(filepath.Join(workDir, useDir))
		if err != nil {
			return nil, fmt.Errorf("resolve workspace use path %q: %w", useDir, err)
		}

		if _, ok := seen[moduleDir]; ok {
			continue
		}

		modulePath, err := parseModulePath(moduleDir)
		if err != nil {
			return nil, err
		}

		modules = append(modules, moduleRoot{
			dir:        moduleDir,
			modulePath: modulePath,
		})
		seen[moduleDir] = struct{}{}
	}

	if len(modules) == 0 {
		return nil, fmt.Errorf("workspace %q does not declare any module use paths", workDir)
	}

	return modules, nil
}

// loadModuleRoots loads one module root from a go.mod directory.
func loadModuleRoots(moduleDir string) ([]moduleRoot, error) {
	modulePath, err := parseModulePath(moduleDir)
	if err != nil {
		return nil, err
	}

	return []moduleRoot{{
		dir:        moduleDir,
		modulePath: modulePath,
	}}, nil
}

// parseGoWorkUseDirs extracts `use` directives from a go.work file.
func parseGoWorkUseDirs(workDir string) ([]string, error) {
	file, err := os.Open(filepath.Join(workDir, "go.work"))
	if err != nil {
		return nil, fmt.Errorf("open go.work: %w", err)
	}
	defer file.Close()

	var paths []string
	inUseBlock := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := stripGoDirectiveComment(scanner.Text())
		if line == "" {
			continue
		}

		switch {
		case inUseBlock:
			if line == ")" {
				inUseBlock = false
				continue
			}

			paths = append(paths, trimDirectiveValue(line))

		case line == "use (" || line == "use(":
			inUseBlock = true

		case strings.HasPrefix(line, "use "):
			paths = append(paths, trimDirectiveValue(strings.TrimSpace(strings.TrimPrefix(line, "use"))))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read go.work: %w", err)
	}

	return paths, nil
}

// parseModulePath extracts the module path from a go.mod file.
func parseModulePath(moduleDir string) (string, error) {
	file, err := os.Open(filepath.Join(moduleDir, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("open go.mod: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := stripGoDirectiveComment(scanner.Text())
		if !strings.HasPrefix(line, "module ") {
			continue
		}

		modulePath := trimDirectiveValue(strings.TrimSpace(strings.TrimPrefix(line, "module")))
		if modulePath == "" {
			break
		}

		return modulePath, nil
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}

	return "", fmt.Errorf("module path not found in %q", filepath.Join(moduleDir, "go.mod"))
}

// stripGoDirectiveComment removes trailing line comments from go.work/go.mod directives.
func stripGoDirectiveComment(line string) string {
	if idx := strings.Index(line, "//"); idx >= 0 {
		line = line[:idx]
	}

	return strings.TrimSpace(line)
}

// trimDirectiveValue trims whitespace and optional quotes.
func trimDirectiveValue(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"`)
}

// resolvePackage finds and caches one package from the configured lookup roots.
func resolvePackage(lookupRoots []lookupRoot, importPath string, cache map[packageCacheKey]packageInfo) (packageInfo, lookupRoot, error) {
	var failures []string
	for _, lookupRoot := range lookupRoots {
		info, err := resolvePackageInRoot(lookupRoot, importPath, cache)
		if err == nil {
			return info, lookupRoot, nil
		}

		failures = append(failures, fmt.Sprintf("%s: %v", lookupRoot.path, err))
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
		lookupPath: lookupRoot.path,
		importPath: importPath,
	}
	if info, ok := cache[key]; ok {
		return info, nil
	}

	module, relPath, ok := lookupRoot.bestModuleForImport(importPath)
	if !ok {
		return packageInfo{}, fmt.Errorf("no module matches import path %q", importPath)
	}

	dir := module.dir
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

// bestModuleForImport returns the longest matching module path for one import.
func (lookupRoot lookupRoot) bestModuleForImport(importPath string) (moduleRoot, string, bool) {
	bestIndex := -1
	bestRel := ""
	bestLen := -1
	for idx, module := range lookupRoot.modules {
		relPath, ok := importPathWithinModule(importPath, module.modulePath)
		if !ok {
			continue
		}

		if len(module.modulePath) <= bestLen {
			continue
		}

		bestIndex = idx
		bestRel = relPath
		bestLen = len(module.modulePath)
	}

	if bestIndex < 0 {
		return moduleRoot{}, "", false
	}

	return lookupRoot.modules[bestIndex], bestRel, true
}

// importPathWithinModule reports whether an import path belongs to a module root.
func importPathWithinModule(importPath, modulePath string) (string, bool) {
	if importPath == modulePath {
		return "", true
	}
	if !strings.HasPrefix(importPath, modulePath+"/") {
		return "", false
	}

	return strings.TrimPrefix(importPath, modulePath+"/"), true
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

	return defaultImportName(importPath), nil
}

// defaultImportName returns the most likely default package identifier for one import path.
func defaultImportName(importPath string) string {
	name := filepath.Base(importPath)
	if idx := strings.LastIndex(name, ".v"); idx > 0 {
		suffix := name[idx+2:]
		if suffix != "" && allDigits(suffix) {
			name = name[:idx]
		}
	}

	return name
}

// allDigits reports whether s contains only ASCII digits.
func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}

	return s != ""
}

// errorsIsNotExist reports whether err is an os.ErrNotExist condition.
func errorsIsNotExist(err error) bool {
	return err != nil && os.IsNotExist(err)
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
	methods := make([]methodDecl, 0, len(pkg.methods[typeName])+len(pkg.interfaceMethods[typeName]))
	seen := map[string]struct{}{}

	pkg.appendDeclaredMethods(typeName, &methods, seen)
	if includeEmbedded {
		pkg.appendEmbeddedMethods(typeName, &methods, seen, map[string]struct{}{})
	}

	return methods
}

// appendDeclaredMethods appends methods declared directly on the named type.
func (pkg *sourcePackage) appendDeclaredMethods(typeName string, methods *[]methodDecl, seen map[string]struct{}) {
	for _, method := range pkg.methods[typeName] {
		pkg.appendMethod(method, methods, seen)
	}
	for _, method := range pkg.interfaceMethods[typeName] {
		pkg.appendMethod(method, methods, seen)
	}
}

// appendEmbeddedMethods walks embedded local types and appends their methods.
func (pkg *sourcePackage) appendEmbeddedMethods(
	typeName string,
	methods *[]methodDecl,
	seen map[string]struct{},
	visiting map[string]struct{},
) {
	if _, ok := visiting[typeName]; ok {
		return
	}
	visiting[typeName] = struct{}{}
	defer delete(visiting, typeName)

	typeSpec := pkg.typeSpecs[typeName]
	if typeSpec == nil {
		return
	}

	switch typ := typeSpec.Type.(type) {
	case *ast.StructType:
		for _, field := range typ.Fields.List {
			if len(field.Names) != 0 {
				continue
			}

			embeddedType, ok := localTypeName(field.Type)
			if !ok {
				continue
			}

			pkg.appendDeclaredMethods(embeddedType, methods, seen)
			pkg.appendEmbeddedMethods(embeddedType, methods, seen, visiting)
		}

	case *ast.InterfaceType:
		for _, field := range typ.Methods.List {
			if len(field.Names) != 0 {
				continue
			}

			embeddedType, ok := localTypeName(field.Type)
			if !ok {
				continue
			}

			pkg.appendDeclaredMethods(embeddedType, methods, seen)
			pkg.appendEmbeddedMethods(embeddedType, methods, seen, visiting)
		}
	}
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
