package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	interfacify "github.com/thetechpanda/interfacify/pkg"
)

// TestParseFlags verifies CLI parsing for defaults and custom naming flags.
func TestParseFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want interfacify.Config
	}{
		{
			name: "defaults",
			want: interfacify.Config{
				PathsList:       ".",
				OutputFile:      "generated.go",
				OutputPkg:       "output",
				IncludeEmbedded: true,
			},
		},
		{
			name: "custom values",
			args: []string{
				"-paths", "./svc-a, ./svc-b",
				"-structs", "example.com/project/service.Runner",
				"-ofile", "runner_gen.go",
				"-pkg", "service",
				"-prefix", "Generated",
				"-suffix", "Contract",
				"-deep=false",
			},
			want: interfacify.Config{
				PathsList:       "./svc-a, ./svc-b",
				StructsList:     "example.com/project/service.Runner",
				OutputFile:      "runner_gen.go",
				OutputPkg:       "service",
				IncludeEmbedded: false,
				Prefix:          "Generated",
				Suffix:          "Contract",
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseFlags(test.args)
			if err != nil {
				t.Fatalf("parseFlags() error = %v", err)
			}

			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("parseFlags() = %#v, want %#v", got, test.want)
			}
		})
	}
}

// TestParseFlagsHelp verifies that help requests are surfaced to main.
func TestParseFlagsHelp(t *testing.T) {
	t.Parallel()

	_, err := parseFlags([]string{"-help"})
	if err == nil {
		t.Fatal("parseFlags() error = nil, want flag.ErrHelp")
	}

	if err != flag.ErrHelp {
		t.Fatalf("parseFlags() error = %v, want %v", err, flag.ErrHelp)
	}
}

// TestPrintUsage verifies that usage text exposes the public CLI surface.
func TestPrintUsage(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	printUsage(&output)

	text := output.String()
	for _, want := range []string{
		"Usage: interfacify [flags]",
		"-paths",
		"-prefix",
		"-suffix",
		"-structs",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("printUsage() missing %q in output:\n%s", want, text)
		}
	}

	if strings.Contains(text, "-mod") {
		t.Fatalf("printUsage() unexpectedly contains removed -mod flag:\n%s", text)
	}
}

// TestMainWritesOutput verifies that main can run the happy path without exiting.
func TestMainWritesOutput(t *testing.T) {
	t.Parallel()

	outputFile := filepath.Join(t.TempDir(), "generated.go")
	oldArgs := os.Args
	t.Cleanup(func() {
		os.Args = oldArgs
	})

	os.Args = []string{
		"interfacify",
		"-paths", "./pkg/test_data/_basic",
		"-structs", "example.com/interfacify-basic/examples.A",
		"-ofile", outputFile,
		"-pkg", "examples",
		"-prefix", "Prefix",
		"-suffix", "Suffix",
		"-deep=false",
	}

	main()

	got, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("os.ReadFile(output) error = %v", err)
	}

	want, err := os.ReadFile(filepath.Join("pkg", "test_data", "_basic", "expected_shallow.golden"))
	if err != nil {
		t.Fatalf("os.ReadFile(expected) error = %v", err)
	}

	if string(got) != string(want) {
		t.Fatalf("main() wrote unexpected output\n\ngot:\n%s\n\nwant:\n%s", got, want)
	}
}
