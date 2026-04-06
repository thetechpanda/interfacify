<p align="center">
  <img src="assets/logo.png" alt="interfacify logo" width="220">
</p>

# interfacify

[![Test](https://github.com/thetechpanda/interfacify/actions/workflows/go.yaml/badge.svg?branch=main)](https://github.com/thetechpanda/interfacify/actions/workflows/go.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/thetechpanda/interfacify)](https://goreportcard.com/report/github.com/thetechpanda/interfacify)
[![Go Reference](https://pkg.go.dev/badge/github.com/thetechpanda/interfacify.svg)](https://pkg.go.dev/github.com/thetechpanda/interfacify)
[![Release](https://img.shields.io/github/release/thetechpanda/interfacify.svg?style=flat-square)](https://github.com/thetechpanda/interfacify/releases)
![Dependencies](https://img.shields.io/badge/Go_Dependencies-_None_-green.svg)

`interfacify` generates Go interfaces from one or more concrete types.

It resolves packages from one or more Go module or workspace paths, collects exported methods for each requested type, preserves doc comments when it can resolve them from source, and writes formatted Go code to `-ofile`.

With `-deep=true`, exported methods promoted through embedded local structs or interfaces are included in the generated interface.

## Install

Install the command directly:

```bash
go install github.com/thetechpanda/interfacify@latest
```

Or add it as a Go tool dependency in the module where you want to use it:

```bash
go get -tool github.com/thetechpanda/interfacify@latest
go tool interfacify -help
```

That pins the tool in your `go.mod`, so the project can run a consistent version with `go tool interfacify`.
This repository currently requires Go `1.26.1`, so the `go tool` flow assumes your module is already using Go `1.26.1+` or can auto-switch toolchains.

Or build it from this repository:

```bash
go build -o interfacify .
```

To inspect the CLI without building a binary:

```bash
go run . -help
```

## Usage

```bash
interfacify \
  -paths . \
  -structs example.com/project/examples.A,\
    example.com/project/examples.B \
  -ofile generated.go \
  -pkg examples \
  -suffix Interface \
  -deep=true
```

The same invocation works through `go tool` once the tool is added to your module:

```bash
go tool interfacify \
  -paths . \
  -structs example.com/project/examples.A,\
    example.com/project/examples.B \
  -ofile generated.go \
  -pkg examples \
  -suffix Interface \
  -deep=true
```

## Flags

| Flag       | Default        | Description                                                                                                        |
| ---------- | -------------- | ------------------------------------------------------------------------------------------------------------------ |
| `-paths`   | `.`            | Comma-separated list of module or workspace paths to search.<br>The first matching path wins for each import path. |
| `-structs` | `""`           | Comma-separated list of fully-qualified type names to convert.                                                     |
| `-ofile`   | `generated.go` | Output file path for the generated Go source.                                                                      |
| `-pkg`     | `output`       | Package name to use in the generated file.                                                                         |
| `-prefix`  | `""`           | Optional prefix for generated interface names.                                                                     |
| `-suffix`  | `""`           | Optional suffix for generated interface names.                                                                     |
| `-deep`    | `true`         | Include exported methods promoted through embedded local<br>structs or interfaces.                                 |

When generating into the same package as the source types, use `-prefix` or `-suffix` to avoid redeclaring the original type names.

## Examples

All examples below assume you are running from the repository root and use the fixture modules under `pkg/test_data`.

### Basic embedded methods

Generate two interfaces from the `_basic` fixture, including exported methods promoted through embedding:

```bash
go run . \
  -paths ./pkg/test_data/_basic \
  -structs example.com/interfacify-basic/examples.A,\
    example.com/interfacify-basic/examples.B \
  -ofile ./tmp/basic_deep.go \
  -pkg examples \
  -prefix Prefix \
  -suffix Suffix \
  -deep=true
```

This matches [pkg/test_data/_basic/expected_deep.golden](pkg/test_data/_basic/expected_deep.golden).

### Shallow generation

Generate only methods declared directly on `A` from the same fixture:

```bash
go run . \
  -paths ./pkg/test_data/_basic \
  -structs example.com/interfacify-basic/examples.A \
  -ofile ./tmp/basic_shallow.go \
  -pkg examples \
  -prefix Prefix \
  -suffix Suffix \
  -deep=false
```

This matches [pkg/test_data/_basic/expected_shallow.golden](pkg/test_data/_basic/expected_shallow.golden).

### Package path vs module path

`-structs` must point at the package that owns the type, not just the module root. In `_basic`, `A` lives in the `examples` package:

```bash
go run . \
  -paths ./pkg/test_data/_basic \
  -structs example.com/interfacify-basic/examples.A \
  -ofile ./tmp/basic_single.go \
  -pkg main
```

That generates:

```go
package main

// A is an interface matching example.com/interfacify-basic/examples.A
type A interface {
 HasA() bool
 HasC() bool
}
```

### Imported types in method signatures

Generate an interface from the `_imports` fixture and keep the output in the source package:

```bash
go run . \
  -paths ./pkg/test_data/_imports \
  -structs example.com/interfacify-imports/service.Runner \
  -ofile ./tmp/imports.go \
  -pkg service \
  -prefix Prefix \
  -suffix Suffix
```

This matches [pkg/test_data/_imports/expected.golden](pkg/test_data/_imports/expected.golden).

### Generate into a different package

If a method signature uses exported types, `interfacify` qualifies source-package types so you can emit the interface into another package:

```bash
go run . \
  -paths ./pkg/test_data/_different_pkg \
  -structs example.com/interfacify-differentpkg/service.Runner \
  -ofile ./tmp/different_pkg.go \
  -pkg generated \
  -prefix Prefix \
  -suffix Suffix
```

This matches [pkg/test_data/_different_pkg/expected.golden](pkg/test_data/_different_pkg/expected.golden).

### Nested embedded methods

Generate an interface from the `_nested` fixture to include methods promoted through multiple embedded layers:

```bash
go run . \
  -paths ./pkg/test_data/_nested \
  -structs example.com/interfacify-nested/nested.Top \
  -ofile ./tmp/nested.go \
  -pkg nested \
  -prefix Prefix \
  -suffix Suffix
```

This matches [pkg/test_data/_nested/expected.golden](pkg/test_data/_nested/expected.golden).

### Multifile package

Generate an interface from a package spread across multiple files:

```bash
go run . \
  -paths ./pkg/test_data/_multifile \
  -structs example.com/interfacify-multifile/service.Worker \
  -ofile ./tmp/multifile.go \
  -pkg service \
  -prefix Prefix \
  -suffix Suffix
```

This matches [pkg/test_data/_multifile/expected.golden](pkg/test_data/_multifile/expected.golden).

### Generic interface and struct

Generate interfaces from the `_generics` fixture. The generated interfaces preserve source type parameters:

```bash
go run . \
  -paths ./pkg/test_data/_generics \
  -structs example.com/interfacify-generics/service.Reader,\
    example.com/interfacify-generics/service.Loader \
  -ofile ./tmp/generics.go \
  -pkg service \
  -prefix Prefix \
  -suffix Suffix
```

This matches [pkg/test_data/_generics/expected.golden](pkg/test_data/_generics/expected.golden).

### Multiple type parameters

Generate interfaces from the `_generics_multi` fixture to preserve multiple type parameters and promoted methods from an embedded generic interface:

```bash
go run . \
  -paths ./pkg/test_data/_generics_multi \
  -structs example.com/interfacify-generics-multi/service.Pair,\
    example.com/interfacify-generics-multi/service.Entry \
  -ofile ./tmp/generics_multi.go \
  -pkg service \
  -prefix Prefix \
  -suffix Suffix
```

This matches [pkg/test_data/_generics_multi/expected.golden](pkg/test_data/_generics_multi/expected.golden).

### Multiple lookup paths

Search more than one local module path. The first path that can resolve the import path wins:

```bash
go run . \
  -paths ./pkg/test_data/_imports,./pkg/test_data/_basic \
  -structs example.com/interfacify-basic/examples.A \
  -ofile ./tmp/multi_paths.go \
  -pkg examples \
  -prefix Prefix \
  -suffix Suffix \
  -deep=false
```

Even though `_imports` is listed first, the generator falls through to `_basic` because that is the first path containing `example.com/interfacify-basic/examples`.

## Constraints

- Use `-paths` to tell the generator which local modules or workspaces it may search. The default is the current directory.
- `-structs` entries must be fully-qualified import paths followed by the type name.
- Exported source-package types are qualified with the source package import when generating into a different package.
- If a method signature refers to unexported local package types, the generated file must stay in the same package as the source.
- Embedded method promotion only follows local embedded structs and interfaces.

## Tests

```bash
go test ./...
```
