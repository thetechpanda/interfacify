package decoders

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LookupRoot stores one configured lookup path and the modules it exposes.
type LookupRoot struct {
	Path    string
	Modules []ModuleRoot
}

// ModuleRoot stores one resolved module root available from a lookup path.
type ModuleRoot struct {
	Dir        string
	ModulePath string
}

// ResolveLookupPaths validates, resolves, and deduplicates configured lookup paths.
func ResolveLookupPaths(pathsList string) ([]string, error) {
	pathsList = strings.TrimSpace(pathsList)
	if pathsList == "" {
		pathsList = "."
	}

	rawPaths := strings.Split(pathsList, ",")
	lookupPaths := make([]string, 0, len(rawPaths))
	seen := make(map[string]struct{}, len(rawPaths))
	for _, rawPath := range rawPaths {
		rawPath = strings.TrimSpace(rawPath)
		if rawPath == "" {
			continue
		}

		resolvedPath, err := ResolveDir(rawPath)
		if err != nil {
			return nil, err
		}

		if _, ok := seen[resolvedPath]; ok {
			continue
		}

		lookupPaths = append(lookupPaths, resolvedPath)
		seen[resolvedPath] = struct{}{}
	}

	if len(lookupPaths) == 0 {
		return nil, fmt.Errorf("no lookup paths provided")
	}

	return lookupPaths, nil
}

// ResolveDir validates and absolutizes one directory path.
func ResolveDir(dir string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", dir, err)
	}

	info, err := os.Stat(absDir)
	if err != nil {
		return "", fmt.Errorf("stat %q: %w", absDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", absDir)
	}

	return absDir, nil
}

// BuildLookupRoots resolves configured lookup paths into module roots.
func BuildLookupRoots(lookupPaths []string) ([]LookupRoot, error) {
	roots := make([]LookupRoot, 0, len(lookupPaths))
	for _, lookupPath := range lookupPaths {
		root, err := BuildLookupRoot(lookupPath)
		if err != nil {
			return nil, err
		}

		roots = append(roots, root)
	}

	return roots, nil
}

// BuildLookupRoot resolves one lookup path into a module or workspace root.
func BuildLookupRoot(lookupPath string) (LookupRoot, error) {
	rootDir, rootKind, err := FindLookupEnvironment(lookupPath)
	if err != nil {
		return LookupRoot{}, err
	}

	var modules []ModuleRoot
	switch rootKind {
	case "work":
		modules, err = LoadWorkspaceModules(rootDir)
	case "mod":
		modules, err = LoadModuleRoots(rootDir)
	default:
		err = fmt.Errorf("unsupported lookup root type %q", rootKind)
	}
	if err != nil {
		return LookupRoot{}, err
	}

	return LookupRoot{
		Path:    lookupPath,
		Modules: modules,
	}, nil
}

// FindLookupEnvironment finds the effective module or workspace root for one path.
func FindLookupEnvironment(startDir string) (string, string, error) {
	dir := startDir
	var moduleDir string
	for {
		workFile := filepath.Join(dir, "go.work")
		if _, err := os.Stat(workFile); err == nil {
			return dir, "work", nil
		} else if err != nil && !os.IsNotExist(err) {
			return "", "", fmt.Errorf("stat %q: %w", workFile, err)
		}

		modFile := filepath.Join(dir, "go.mod")
		if moduleDir == "" {
			if _, err := os.Stat(modFile); err == nil {
				moduleDir = dir
			} else if err != nil && !os.IsNotExist(err) {
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

// LoadWorkspaceModules loads all module roots referenced by a go.work file.
func LoadWorkspaceModules(workDir string) ([]ModuleRoot, error) {
	useDirs, err := ParseGoWorkUseDirs(workDir)
	if err != nil {
		return nil, err
	}

	modules := make([]ModuleRoot, 0, len(useDirs))
	seen := make(map[string]struct{}, len(useDirs))
	for _, useDir := range useDirs {
		moduleDir, err := ResolveDir(filepath.Join(workDir, useDir))
		if err != nil {
			return nil, fmt.Errorf("resolve workspace use path %q: %w", useDir, err)
		}

		if _, ok := seen[moduleDir]; ok {
			continue
		}

		modulePath, err := ParseModulePath(moduleDir)
		if err != nil {
			return nil, err
		}

		modules = append(modules, ModuleRoot{
			Dir:        moduleDir,
			ModulePath: modulePath,
		})
		seen[moduleDir] = struct{}{}
	}

	if len(modules) == 0 {
		return nil, fmt.Errorf("workspace %q does not declare any module use paths", workDir)
	}

	return modules, nil
}

// LoadModuleRoots loads one module root from a go.mod directory.
func LoadModuleRoots(moduleDir string) ([]ModuleRoot, error) {
	modulePath, err := ParseModulePath(moduleDir)
	if err != nil {
		return nil, err
	}

	return []ModuleRoot{{
		Dir:        moduleDir,
		ModulePath: modulePath,
	}}, nil
}

// ParseGoWorkUseDirs extracts `use` directives from a go.work file.
func ParseGoWorkUseDirs(workDir string) ([]string, error) {
	file, err := os.Open(filepath.Join(workDir, "go.work"))
	if err != nil {
		return nil, fmt.Errorf("open go.work: %w", err)
	}
	defer file.Close()

	var paths []string
	inUseBlock := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := StripGoDirectiveComment(scanner.Text())
		if line == "" {
			continue
		}

		switch {
		case inUseBlock:
			if line == ")" {
				inUseBlock = false
				continue
			}

			paths = append(paths, TrimDirectiveValue(line))

		case line == "use (" || line == "use(":
			inUseBlock = true

		case strings.HasPrefix(line, "use "):
			paths = append(paths, TrimDirectiveValue(strings.TrimSpace(strings.TrimPrefix(line, "use"))))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read go.work: %w", err)
	}

	return paths, nil
}

// ParseModulePath extracts the module path from a go.mod file.
func ParseModulePath(moduleDir string) (string, error) {
	file, err := os.Open(filepath.Join(moduleDir, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("open go.mod: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := StripGoDirectiveComment(scanner.Text())
		if !strings.HasPrefix(line, "module ") {
			continue
		}

		modulePath := TrimDirectiveValue(strings.TrimSpace(strings.TrimPrefix(line, "module")))
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

// StripGoDirectiveComment removes trailing line comments from go.work/go.mod directives.
func StripGoDirectiveComment(line string) string {
	if idx := strings.Index(line, "//"); idx >= 0 {
		line = line[:idx]
	}

	return strings.TrimSpace(line)
}

// TrimDirectiveValue trims whitespace and optional quotes.
func TrimDirectiveValue(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"`)
}

// BestModuleForImport returns the longest matching module path for one import.
func (lookupRoot LookupRoot) BestModuleForImport(importPath string) (ModuleRoot, string, bool) {
	bestIndex := -1
	bestRel := ""
	bestLen := -1
	for idx, module := range lookupRoot.Modules {
		relPath, ok := ImportPathWithinModule(importPath, module.ModulePath)
		if !ok {
			continue
		}

		if len(module.ModulePath) <= bestLen {
			continue
		}

		bestIndex = idx
		bestRel = relPath
		bestLen = len(module.ModulePath)
	}

	if bestIndex < 0 {
		return ModuleRoot{}, "", false
	}

	return lookupRoot.Modules[bestIndex], bestRel, true
}

// ImportPathWithinModule reports whether an import path belongs to a module root.
func ImportPathWithinModule(importPath, modulePath string) (string, bool) {
	if importPath == modulePath {
		return "", true
	}
	if !strings.HasPrefix(importPath, modulePath+"/") {
		return "", false
	}

	return strings.TrimPrefix(importPath, modulePath+"/"), true
}

// DefaultImportName returns the most likely default package identifier for one import path.
func DefaultImportName(importPath string) string {
	name := filepath.Base(importPath)
	if idx := strings.LastIndex(name, ".v"); idx > 0 {
		suffix := name[idx+2:]
		if suffix != "" && AllDigits(suffix) {
			name = name[:idx]
		}
	}

	return name
}

// AllDigits reports whether s contains only ASCII digits.
func AllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}

	return s != ""
}
