package interfacify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config stores the CLI options used to generate interfaces.
type Config struct {
	// PathsList is a comma-separated list of module or workspace directories to search.
	// The first matching path wins for each requested import path.
	PathsList string
	// StructsList contains the fully-qualified target type names.
	StructsList string
	// OutputFile is where the generated code is written.
	OutputFile string
	// OutputPkg is the package name used in the generated file.
	OutputPkg string
	// IncludeEmbedded controls whether embedded methods are included.
	IncludeEmbedded bool
	// Prefix specify generated interfaces prefix
	Prefix string
	// Suffix specify generated interfaces suffix
	Suffix string
}

// resolveLookupPaths returns the directories used for package resolution.
func (cfg Config) resolveLookupPaths() ([]string, error) {
	pathsList := strings.TrimSpace(cfg.PathsList)
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

		resolvedPath, err := resolveDir(rawPath)
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

// resolveDir validates and absolutizes one directory path.
func resolveDir(dir string) (string, error) {
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

// Run generates interfaces and writes the result to disk.
func Run(cfg Config) error {
	output, err := Generate(cfg)
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}

	if err := os.WriteFile(cfg.OutputFile, output, 0o644); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	return nil
}
