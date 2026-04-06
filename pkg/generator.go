package interfacify

import (
	"fmt"
	"go/format"
	"sort"
	"strconv"
	"strings"

	decoders "github.com/thetechpanda/interfacify/pkg/decoders"
	encoders "github.com/thetechpanda/interfacify/pkg/encoders"
)

// target identifies one source type to convert into an interface.
type target struct {
	// fullName is the fully-qualified type name from the CLI.
	fullName string
	// importPath is the package import path that owns the type.
	importPath string
	// typeName is the unqualified source type name.
	typeName string
}

// Generate loads the requested types and renders the output file.
func Generate(cfg Config) ([]byte, error) {
	lookupPaths, err := cfg.resolveLookupPaths()
	if err != nil {
		return nil, err
	}

	lookupRoots, err := decoders.BuildLookupRoots(lookupPaths)
	if err != nil {
		return nil, err
	}

	targets, importPaths, err := parseTargets(cfg.StructsList)
	if err != nil {
		return nil, err
	}

	packageCache := map[packageCacheKey]packageInfo{}
	loaded := make(map[string]*sourcePackage, len(importPaths))
	for _, importPath := range importPaths {
		sourcePkg, err := loadSourcePackage(lookupRoots, importPath, packageCache)
		if err != nil {
			return nil, err
		}

		loaded[importPath] = sourcePkg
	}

	blocks := make([]string, 0, len(targets))
	imports := map[string]string{}
	for _, target := range targets {
		sourcePkg := loaded[target.importPath]
		if sourcePkg == nil {
			return nil, fmt.Errorf("package %q not found", target.importPath)
		}

		block, blockImports, err := renderInterface(sourcePkg, target, cfg.OutputPkg, cfg.IncludeEmbedded, cfg.Prefix, cfg.Suffix)
		if err != nil {
			return nil, err
		}

		if err := mergeImports(imports, blockImports); err != nil {
			return nil, err
		}

		blocks = append(blocks, block)
	}

	var output strings.Builder
	output.WriteString("package ")
	output.WriteString(cfg.OutputPkg)
	output.WriteString("\n")

	if len(imports) > 0 {
		aliases := make([]string, 0, len(imports))
		for alias := range imports {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)

		output.WriteString("\nimport (\n")
		for _, alias := range aliases {
			output.WriteString("\t")
			output.WriteString(alias)
			output.WriteString(" ")
			output.WriteString(strconv.Quote(imports[alias]))
			output.WriteString("\n")
		}
		output.WriteString(")\n")
	}

	if len(blocks) > 0 {
		output.WriteString("\n")
		output.WriteString(strings.Join(blocks, "\n\n"))
		output.WriteString("\n")
	}

	return format.Source([]byte(output.String()))
}

// parseTargets parses the comma-separated struct list from the CLI.
func parseTargets(structsList string) ([]target, []string, error) {
	rawTargets := strings.Split(structsList, ",")
	targets := make([]target, 0, len(rawTargets))
	importPaths := make([]string, 0, len(rawTargets))
	seenImports := make(map[string]struct{}, len(rawTargets))

	for _, rawTarget := range rawTargets {
		rawTarget = strings.TrimSpace(rawTarget)
		if rawTarget == "" {
			continue
		}

		idx := strings.LastIndexByte(rawTarget, '.')
		if idx <= 0 || idx == len(rawTarget)-1 {
			return nil, nil, fmt.Errorf("invalid struct %q", rawTarget)
		}

		target := target{
			fullName:   rawTarget,
			importPath: rawTarget[:idx],
			typeName:   rawTarget[idx+1:],
		}
		targets = append(targets, target)

		if _, ok := seenImports[target.importPath]; ok {
			continue
		}

		importPaths = append(importPaths, target.importPath)
		seenImports[target.importPath] = struct{}{}
	}

	if len(targets) == 0 {
		return nil, nil, fmt.Errorf("no structs provided")
	}

	return targets, importPaths, nil
}

// renderInterface renders one interface block for a single source type.
func renderInterface(
	sourcePkg *sourcePackage,
	target target,
	outputPkg string,
	includeEmbedded bool,
	prefix, suffix string,
) (string, map[string]string, error) {
	typeSpec := sourcePkg.typeSpecs[target.typeName]
	if typeSpec == nil {
		return "", nil, fmt.Errorf("type %q not found in package %q", target.typeName, target.importPath)
	}

	methods := sourcePkg.collectMethods(target.typeName, includeEmbedded)
	renderer := signatureRenderer{
		outputPkg:   outputPkg,
		pkg:         sourcePkg,
		usedImports: map[string]string{},
	}
	typeParams, err := renderer.renderTypeParams(typeSpec.TypeParams)
	if err != nil {
		return "", nil, fmt.Errorf("rendering type parameters for %s: %w", target.typeName, err)
	}

	var block strings.Builder
	ifaceName := prefix + target.typeName + suffix
	encoders.WriteCommentLines(&block, fmt.Sprintf("%s is an interface matching %s", ifaceName, target.fullName), "")

	if typeDoc := sourcePkg.typeDocs[target.typeName]; typeDoc != "" {
		block.WriteString("//\n")
		encoders.WriteCommentLines(&block, typeDoc, "")
	}

	block.WriteString("type ")
	block.WriteString(ifaceName)
	block.WriteString(typeParams)
	block.WriteString(" interface {\n")
	for _, method := range methods {
		if doc := sourcePkg.docForMethod(method); doc != "" {
			encoders.WriteCommentLines(&block, doc, "\t")
		}

		signature, err := renderer.renderMethod(method)
		if err != nil {
			return "", nil, fmt.Errorf("rendering %s.%s: %w", target.typeName, method.name, err)
		}

		block.WriteString("\t")
		block.WriteString(signature)
		block.WriteString("\n")
	}
	block.WriteString("}")

	return block.String(), renderer.usedImports, nil
}

// mergeImports merges block-level imports into the output import set.
func mergeImports(dst, src map[string]string) error {
	for alias, importPath := range src {
		if existing, ok := dst[alias]; ok && existing != importPath {
			return fmt.Errorf("import name %q is used by both %q and %q", alias, existing, importPath)
		}

		dst[alias] = importPath
	}

	return nil
}
