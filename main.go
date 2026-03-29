// # interfacify
//
// Turn a structs into interfaces.
// When a struct embeds a struct or interface that exports methods these methods are added to the interface this behaviour is controlled via -deep=true (default).
//
// Assuming the following structs:
//
//	package examples
//
//	// A handles A and C flags lookup
//	type A struct{ *C }
//
//	// HasA true if A is true
//	func (*A) HasA() bool { return true }
//
//	type iC interface{ HasC() bool }
//
//	// B handles B and C flags lookup
//	type B struct{ iC }
//
//	// HasB true if B is true
//	func (*B) HasB() bool { return true }
//
//	type C struct{}
//
//	// HasC true if C is true
//	func (*C) HasC() bool { return true }
//
// Running the following command:
//
//	interfacify -structs git.host/examples.A,git.host/examples.B -ofile generated.go -deep -pkg examples -suffix Interface
//
// # Results in
//
//	package examples
//
//	// AInterface is an interface matching git.host/examples.A
//	//
//	// A handles A and C flags lookup
//	type AInterface interface {
//	    // HasA true if A is true
//	    HasA() bool
//	    // HasC true if C is true
//	    HasC() bool
//	}
//
//	// BInterface is an interface matching git.host/examples.B
//	//
//	// B handles B and C flags lookup
//	type BInterface interface {
//	    // HasB true if B is true
//	    HasB() bool
//	    // HasC true if C is true
//	    HasC() bool
//	}
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	interfacify "github.com/thetechpanda/interfacify/pkg"
)

// defaultConfig returns the default CLI configuration.
func defaultConfig() interfacify.Config {
	return interfacify.Config{
		PathsList:       ".",
		OutputFile:      "generated.go",
		OutputPkg:       "output",
		IncludeEmbedded: true,
	}
}

// newFlagSet binds CLI flags onto the provided config.
func newFlagSet(cfg *interfacify.Config, output io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet("interfacify", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.StringVar(&cfg.PathsList, "paths", cfg.PathsList, "comma separated list of module or workspace paths to search")
	flags.StringVar(&cfg.StructsList, "structs", cfg.StructsList, "comma separated list of structs, they must include full go module")
	flags.StringVar(&cfg.OutputFile, "ofile", cfg.OutputFile, "specify output file")
	flags.StringVar(&cfg.OutputPkg, "pkg", cfg.OutputPkg, "specifies the output package name")
	flags.StringVar(&cfg.Prefix, "prefix", cfg.Prefix, "optional prefix for generated interface names")
	flags.StringVar(&cfg.Suffix, "suffix", cfg.Suffix, "optional suffix for generated interface names")
	flags.BoolVar(&cfg.IncludeEmbedded, "deep", cfg.IncludeEmbedded, "if true when a struct embeds a struct or interface that exports methods these methods are added to the interface")

	return flags
}

// parseFlags parses CLI arguments into a Config value.
func parseFlags(args []string) (interfacify.Config, error) {
	cfg := defaultConfig()
	flags := newFlagSet(&cfg, io.Discard)

	if err := flags.Parse(args); err != nil {
		return interfacify.Config{}, err
	}

	return cfg, nil
}

// printUsage writes CLI usage text.
func printUsage(w io.Writer) {
	cfg := defaultConfig()
	flags := newFlagSet(&cfg, w)

	fmt.Fprintln(w, "Usage: interfacify [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Generate Go interfaces from one or more concrete types.")
	fmt.Fprintln(w)
	flags.PrintDefaults()
}

// main parses CLI arguments and runs the generator.
func main() {
	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage(os.Stdout)
			return
		}

		fmt.Fprintf(os.Stderr, "failed to parse arguments: %v\n", err)
		os.Exit(1)
	}

	if err := interfacify.Run(cfg); err != nil {
		printUsage(os.Stderr)
		fmt.Fprintf(os.Stderr, "interfacify: %v\n", err)
		os.Exit(1)
	}
}
