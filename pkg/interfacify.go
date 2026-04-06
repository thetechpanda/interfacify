package interfacify

import (
	"fmt"
	"os"
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
