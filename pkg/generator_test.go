package interfacify_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	interfacify "github.com/thetechpanda/interfacify/pkg"
)

// TestGenerate verifies interface generation against fixture outputs.
func TestGenerate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fixture  string
		structs  string
		pkg      string
		deep     bool
		output   string
		expected string
		prefix   string
		suffix   string
	}{
		{
			name:     "deep embedded methods",
			fixture:  "_basic",
			structs:  "example.com/interfacify-basic/examples.A,example.com/interfacify-basic/examples.B",
			pkg:      "examples",
			deep:     true,
			output:   filepath.Join("examples", "generated.go"),
			expected: "expected_deep.golden",
			prefix:   "Prefix",
			suffix:   "Suffix",
		},
		{
			name:     "shallow methods only",
			fixture:  "_basic",
			structs:  "example.com/interfacify-basic/examples.A",
			pkg:      "examples",
			deep:     false,
			output:   filepath.Join("examples", "generated.go"),
			expected: "expected_shallow.golden",
			prefix:   "Prefix",
			suffix:   "Suffix",
		},
		{
			name:     "imports in signatures",
			fixture:  "_imports",
			structs:  "example.com/interfacify-imports/service.Runner",
			pkg:      "service",
			deep:     true,
			output:   filepath.Join("service", "generated.go"),
			expected: "expected.golden",
			prefix:   "Prefix",
			suffix:   "Suffix",
		},
		{
			name:     "different output package qualifies exported local types",
			fixture:  "_different_pkg",
			structs:  "example.com/interfacify-differentpkg/service.Runner",
			pkg:      "generated",
			deep:     true,
			expected: "expected.golden",
			prefix:   "Prefix",
			suffix:   "Suffix",
		},
		{
			name:     "methods are ordered deterministically",
			fixture:  "_ordering",
			structs:  "example.com/interfacify-ordering/service.Contract,example.com/interfacify-ordering/service.Worker",
			pkg:      "service",
			deep:     true,
			output:   filepath.Join("service", "generated.go"),
			expected: "expected.golden",
			prefix:   "Prefix",
			suffix:   "Suffix",
		},
		{
			name:     "nested embedded methods",
			fixture:  "_nested",
			structs:  "example.com/interfacify-nested/nested.Top",
			pkg:      "nested",
			deep:     true,
			output:   filepath.Join("nested", "generated.go"),
			expected: "expected.golden",
			prefix:   "Prefix",
			suffix:   "Suffix",
		},
		{
			name:     "multifile package with complex signatures",
			fixture:  "_multifile",
			structs:  "example.com/interfacify-multifile/service.Worker",
			pkg:      "service",
			deep:     true,
			output:   filepath.Join("service", "generated.go"),
			expected: "expected.golden",
			prefix:   "Prefix",
			suffix:   "Suffix",
		},
		{
			name:     "generic struct and interface types",
			fixture:  "_generics",
			structs:  "example.com/interfacify-generics/service.Reader,example.com/interfacify-generics/service.Loader",
			pkg:      "service",
			deep:     true,
			output:   filepath.Join("service", "generated.go"),
			expected: "expected.golden",
			prefix:   "Prefix",
			suffix:   "Suffix",
		},
		{
			name:     "multiple type parameters on struct and interface",
			fixture:  "_generics_multi",
			structs:  "example.com/interfacify-generics-multi/service.Pair,example.com/interfacify-generics-multi/service.Entry",
			pkg:      "service",
			deep:     true,
			output:   filepath.Join("service", "generated.go"),
			expected: "expected.golden",
			prefix:   "Prefix",
			suffix:   "Suffix",
		},
		{
			name:     "different output package qualifies exported local and foreign types",
			fixture:  "_different_pkg_nested",
			structs:  "example.com/interfacify-differentpkgnested/service.Runner",
			pkg:      "generated",
			deep:     true,
			expected: "expected.golden",
			prefix:   "Prefix",
			suffix:   "Suffix",
		},
		{
			name:     "imported embedded struct and interface methods are promoted",
			fixture:  "_foreign_embedded",
			structs:  "example.com/interfacify-foreignembedded/service.Service",
			pkg:      "service",
			deep:     true,
			output:   filepath.Join("service", "generated.go"),
			expected: "expected.golden",
			prefix:   "Prefix",
			suffix:   "Suffix",
		},
		{
			name:     "same package name in different directory still qualifies source-local types",
			fixture:  "_same_pkg_name_different_dir",
			structs:  "example.com/interfacify-samepkg/service.Runner",
			pkg:      "service",
			deep:     true,
			output:   filepath.Join("generated", "service", "generated.go"),
			expected: filepath.Join("generated", "service", "expected.golden"),
			prefix:   "Prefix",
			suffix:   "Suffix",
		},
		{
			name:     "external import keeps declared package name",
			fixture:  "_external_pkg_name/source",
			structs:  "example.com/interfacify-externalpkg/source/service.Runner",
			pkg:      "service",
			deep:     true,
			output:   filepath.Join("service", "generated.go"),
			expected: "expected.golden",
			prefix:   "Prefix",
			suffix:   "Suffix",
		},
		{
			name:     "ambiguous promoted methods are omitted and shallow methods win",
			fixture:  "_embedded_conflicts",
			structs:  "example.com/interfacify-embeddedconflicts/service.Runner",
			pkg:      "service",
			deep:     true,
			output:   filepath.Join("service", "generated.go"),
			expected: "expected.golden",
			prefix:   "Prefix",
			suffix:   "Suffix",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cfg := interfacify.Config{
				PathsList:       fixturePath(test.fixture),
				StructsList:     test.structs,
				OutputFile:      fixturePath(test.fixture, outputPath(test.output, test.expected)),
				OutputPkg:       test.pkg,
				IncludeEmbedded: test.deep,
				Prefix:          test.prefix,
				Suffix:          test.suffix,
			}

			got, err := interfacify.Generate(cfg)
			if err != nil {
				t.Fatalf("Generate() error = %v", err)
			}

			want, err := os.ReadFile(fixturePath(test.fixture, test.expected))
			if err != nil {
				t.Fatalf("os.ReadFile(%q) error = %v", test.expected, err)
			}

			if string(got) != string(want) {
				t.Fatalf("generate() output mismatch\n\ngot:\n%s\n\nwant:\n%s", got, want)
			}
		})
	}
}

// TestGenerateErrors verifies expected generation failures for edge cases.
func TestGenerateErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fixture string
		structs string
		pkg     string
		deep    bool
		errText string
		prefix  string
		suffix  string
	}{
		{
			name:    "different output package with unexported local types in signature",
			fixture: "_pkg_mismatch",
			structs: "example.com/interfacify-pkgmismatch/service.Runner",
			pkg:     "generated",
			deep:    true,
			errText: `output package "generated" differs from source package "service"`,
			prefix:  "Prefix",
			suffix:  "Suffix",
		},
		{
			name:    "conflicting import aliases across target packages",
			fixture: "_import_conflict",
			structs: "example.com/interfacify-conflict/alpha.Alpha,example.com/interfacify-conflict/beta.Beta",
			pkg:     "output",
			deep:    true,
			errText: `import name "foo" is used by both`,
			prefix:  "Prefix",
			suffix:  "Suffix",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cfg := interfacify.Config{
				PathsList:       fixturePath(test.fixture),
				StructsList:     test.structs,
				OutputFile:      fixturePath(test.fixture, "generated.go"),
				OutputPkg:       test.pkg,
				IncludeEmbedded: test.deep,
				Prefix:          test.prefix,
				Suffix:          test.suffix,
			}

			_, err := interfacify.Generate(cfg)
			if err == nil {
				t.Fatalf("generate() error = nil, want %q", test.errText)
			}

			if !strings.Contains(err.Error(), test.errText) {
				t.Fatalf("generate() error = %q, want substring %q", err.Error(), test.errText)
			}
		})
	}
}

// TestRunWritesOutput verifies that run writes the generated file to disk.
func TestRunWritesOutput(t *testing.T) {
	t.Parallel()

	outputFile := filepath.Join(t.TempDir(), "generated.go")
	cfg := interfacify.Config{
		PathsList:       fixturePath("_basic"),
		StructsList:     "example.com/interfacify-basic/examples.A,example.com/interfacify-basic/examples.B",
		OutputFile:      outputFile,
		OutputPkg:       "examples",
		IncludeEmbedded: true,
		Prefix:          "Prefix",
		Suffix:          "Suffix",
	}

	if err := interfacify.Run(cfg); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("os.ReadFile(output) error = %v", err)
	}

	want, err := os.ReadFile(fixturePath("_basic", "expected_deep.golden"))
	if err != nil {
		t.Fatalf("os.ReadFile(expected) error = %v", err)
	}

	if string(got) != string(want) {
		t.Fatalf("Run() wrote unexpected output\n\ngot:\n%s\n\nwant:\n%s", got, want)
	}
}

// TestRunErrors verifies that Run wraps generation and write failures.
func TestRunErrors(t *testing.T) {
	t.Parallel()

	t.Run("write output failure", func(t *testing.T) {
		t.Parallel()

		cfg := interfacify.Config{
			PathsList:       fixturePath("_basic"),
			StructsList:     "example.com/interfacify-basic/examples.A",
			OutputFile:      filepath.Join(t.TempDir(), "missing", "generated.go"),
			OutputPkg:       "examples",
			IncludeEmbedded: false,
			Prefix:          "Prefix",
			Suffix:          "Suffix",
		}

		err := interfacify.Run(cfg)
		if err == nil || !strings.Contains(err.Error(), "write output") {
			t.Fatalf("Run(write failure) = %v, want wrapped write output error", err)
		}
	})

	t.Run("generate failure", func(t *testing.T) {
		t.Parallel()

		cfg := interfacify.Config{
			PathsList:       fixturePath("_basic"),
			StructsList:     "example.com/interfacify-basic/examples.DoesNotExist",
			OutputFile:      filepath.Join(t.TempDir(), "generated.go"),
			OutputPkg:       "examples",
			IncludeEmbedded: false,
		}

		err := interfacify.Run(cfg)
		if err == nil || !strings.Contains(err.Error(), "generate") {
			t.Fatalf("Run(generate failure) = %v, want wrapped generate error", err)
		}
	})
}

// TestGenerateSearchesAcrossPaths verifies that lookup falls back across configured paths.
func TestGenerateSearchesAcrossPaths(t *testing.T) {
	t.Parallel()

	cfg := interfacify.Config{
		PathsList:       strings.Join([]string{fixturePath("_imports"), fixturePath("_basic")}, ","),
		StructsList:     "example.com/interfacify-basic/examples.A",
		OutputFile:      fixturePath("_basic", "examples", "generated.go"),
		OutputPkg:       "examples",
		IncludeEmbedded: false,
		Prefix:          "Prefix",
		Suffix:          "Suffix",
	}

	got, err := interfacify.Generate(cfg)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	want, err := os.ReadFile(fixturePath("_basic", "expected_shallow.golden"))
	if err != nil {
		t.Fatalf("os.ReadFile(expected) error = %v", err)
	}

	if string(got) != string(want) {
		t.Fatalf("generate() output mismatch\n\ngot:\n%s\n\nwant:\n%s", got, want)
	}
}

// outputPath returns the configured output path, or falls back to the expected file path.
func outputPath(outputFile, expectedFile string) string {
	if outputFile != "" {
		return outputFile
	}

	return expectedFile
}

// fixturePath resolves paths inside the package test fixtures.
func fixturePath(parts ...string) string {
	all := append([]string{"test_data"}, parts...)
	return filepath.Join(all...)
}
