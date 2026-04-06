package decoders_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	decoders "github.com/thetechpanda/interfacify/pkg/decoders"
)

func TestDecoderHelpers(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	workspaceDir := filepath.Join(root, "workspace")
	modADir := filepath.Join(workspaceDir, "moda")
	modBDir := filepath.Join(workspaceDir, "modb")
	standaloneDir := filepath.Join(root, "standalone")
	emptyDir := filepath.Join(root, "empty")
	badModDir := filepath.Join(root, "badmod")

	writeTestFile(t, filepath.Join(modADir, "go.mod"), "module example.com/moda\n\ngo 1.26.1\n")
	writeTestFile(t, filepath.Join(modADir, "moda.go"), "package moda\n")
	writeTestFile(t, filepath.Join(modBDir, "go.mod"), "module example.com/modb\n\ngo 1.26.1\n")
	writeTestFile(t, filepath.Join(modBDir, "modb.go"), "package modb\n")
	writeTestFile(t, filepath.Join(workspaceDir, "go.work"), "go 1.26.1\n\nuse (\n\t./moda // first module\n\t\"./modb\"\n\t./moda\n)\n")
	writeTestFile(t, filepath.Join(standaloneDir, "go.mod"), "module example.com/standalone\n\ngo 1.26.1\n")
	writeTestFile(t, filepath.Join(standaloneDir, "standalone.go"), "package standalone\n")
	writeTestFile(t, filepath.Join(badModDir, "go.mod"), "go 1.26.1\n")
	if err := os.MkdirAll(emptyDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(emptyDir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(standaloneDir, "nested", "pkg"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(nested) error = %v", err)
	}

	useDirs, err := decoders.ParseGoWorkUseDirs(workspaceDir)
	if err != nil {
		t.Fatalf("ParseGoWorkUseDirs() error = %v", err)
	}
	if want := []string{"./moda", "./modb", "./moda"}; !reflect.DeepEqual(useDirs, want) {
		t.Fatalf("ParseGoWorkUseDirs() = %#v, want %#v", useDirs, want)
	}

	modules, err := decoders.LoadWorkspaceModules(workspaceDir)
	if err != nil {
		t.Fatalf("LoadWorkspaceModules() error = %v", err)
	}
	if len(modules) != 2 {
		t.Fatalf("LoadWorkspaceModules() len = %d, want 2", len(modules))
	}
	if modules[0].ModulePath != "example.com/moda" || modules[1].ModulePath != "example.com/modb" {
		t.Fatalf("LoadWorkspaceModules() module paths = %#v, want moda/modb", modules)
	}

	workspaceRoot, err := decoders.BuildLookupRoot(workspaceDir)
	if err != nil {
		t.Fatalf("BuildLookupRoot(workspace) error = %v", err)
	}
	if len(workspaceRoot.Modules) != 2 {
		t.Fatalf("BuildLookupRoot(workspace) modules len = %d, want 2", len(workspaceRoot.Modules))
	}

	standaloneRoot, err := decoders.BuildLookupRoot(standaloneDir)
	if err != nil {
		t.Fatalf("BuildLookupRoot(standalone) error = %v", err)
	}
	if len(standaloneRoot.Modules) != 1 || standaloneRoot.Modules[0].ModulePath != "example.com/standalone" {
		t.Fatalf("BuildLookupRoot(standalone) = %#v, want standalone module", standaloneRoot)
	}

	lookupRoots, err := decoders.BuildLookupRoots([]string{workspaceDir, standaloneDir})
	if err != nil {
		t.Fatalf("BuildLookupRoots() error = %v", err)
	}
	if len(lookupRoots) != 2 {
		t.Fatalf("BuildLookupRoots() len = %d, want 2", len(lookupRoots))
	}

	rootDir, rootKind, err := decoders.FindLookupEnvironment(filepath.Join(workspaceDir, "moda"))
	if err != nil {
		t.Fatalf("FindLookupEnvironment(workspace child) error = %v", err)
	}
	if rootDir != workspaceDir || rootKind != "work" {
		t.Fatalf("FindLookupEnvironment(workspace child) = (%q, %q), want (%q, %q)", rootDir, rootKind, workspaceDir, "work")
	}

	rootDir, rootKind, err = decoders.FindLookupEnvironment(filepath.Join(standaloneDir, "nested", "pkg"))
	if err != nil {
		t.Fatalf("FindLookupEnvironment(standalone child) error = %v", err)
	}
	if rootDir != standaloneDir || rootKind != "mod" {
		t.Fatalf("FindLookupEnvironment(standalone child) = (%q, %q), want (%q, %q)", rootDir, rootKind, standaloneDir, "mod")
	}

	modulePath, err := decoders.ParseModulePath(modADir)
	if err != nil {
		t.Fatalf("ParseModulePath() error = %v", err)
	}
	if modulePath != "example.com/moda" {
		t.Fatalf("ParseModulePath() = %q, want %q", modulePath, "example.com/moda")
	}

	moduleRoots, err := decoders.LoadModuleRoots(standaloneDir)
	if err != nil {
		t.Fatalf("LoadModuleRoots() error = %v", err)
	}
	if len(moduleRoots) != 1 || moduleRoots[0].ModulePath != "example.com/standalone" {
		t.Fatalf("LoadModuleRoots() = %#v, want standalone module", moduleRoots)
	}

	lookupPaths, err := decoders.ResolveLookupPaths(strings.Join([]string{standaloneDir, standaloneDir, " "}, ","))
	if err != nil {
		t.Fatalf("ResolveLookupPaths() error = %v", err)
	}
	if want := []string{standaloneDir}; !reflect.DeepEqual(lookupPaths, want) {
		t.Fatalf("ResolveLookupPaths() = %#v, want %#v", lookupPaths, want)
	}

	defaultPaths, err := decoders.ResolveLookupPaths("")
	if err != nil {
		t.Fatalf("ResolveLookupPaths(default) error = %v", err)
	}
	if len(defaultPaths) != 1 {
		t.Fatalf("ResolveLookupPaths(default) len = %d, want 1", len(defaultPaths))
	}

	if _, err := decoders.ResolveLookupPaths(", ,"); err == nil {
		t.Fatal("ResolveLookupPaths() error = nil, want error for empty path list")
	}

	filePath := filepath.Join(root, "file.txt")
	writeTestFile(t, filePath, "not a directory\n")
	if _, err := decoders.ResolveDir(filePath); err == nil {
		t.Fatal("ResolveDir(file) error = nil, want non-directory error")
	}

	if _, _, err := decoders.FindLookupEnvironment(emptyDir); err == nil {
		t.Fatal("FindLookupEnvironment(emptyDir) error = nil, want missing go.mod/go.work error")
	}

	if _, err := decoders.ParseModulePath(badModDir); err == nil {
		t.Fatal("ParseModulePath(badModDir) error = nil, want missing module path error")
	}

	if _, err := decoders.LoadWorkspaceModules(emptyDir); err == nil {
		t.Fatal("LoadWorkspaceModules(emptyDir) error = nil, want error")
	}

	if got := decoders.StripGoDirectiveComment("use ./mod // keep path"); got != "use ./mod" {
		t.Fatalf("StripGoDirectiveComment() = %q, want %q", got, "use ./mod")
	}
	if got := decoders.TrimDirectiveValue(" \"./modb\" "); got != "./modb" {
		t.Fatalf("TrimDirectiveValue() = %q, want %q", got, "./modb")
	}
	if got := decoders.DefaultImportName("example.com/foo.v2"); got != "foo" {
		t.Fatalf("DefaultImportName(v2) = %q, want %q", got, "foo")
	}
	if got := decoders.DefaultImportName("example.com/foo.vbeta"); got != "foo.vbeta" {
		t.Fatalf("DefaultImportName(vbeta) = %q, want %q", got, "foo.vbeta")
	}
	if !decoders.AllDigits("12345") {
		t.Fatal("AllDigits(\"12345\") = false, want true")
	}
	if decoders.AllDigits("12a45") {
		t.Fatal("AllDigits(\"12a45\") = true, want false")
	}
	if decoders.AllDigits("") {
		t.Fatal("AllDigits(\"\") = true, want false")
	}

	if rel, ok := decoders.ImportPathWithinModule("example.com/moda/sub", "example.com/moda"); !ok || rel != "sub" {
		t.Fatalf("ImportPathWithinModule() = (%q, %v), want (%q, %v)", rel, ok, "sub", true)
	}
	if _, ok := decoders.ImportPathWithinModule("example.com/other", "example.com/moda"); ok {
		t.Fatal("ImportPathWithinModule() matched unrelated module")
	}

	bestRoot := decoders.LookupRoot{Modules: []decoders.ModuleRoot{{ModulePath: "example.com/mod"}, {ModulePath: "example.com/mod/sub"}}}
	module, rel, ok := bestRoot.BestModuleForImport("example.com/mod/sub/pkg")
	if !ok || module.ModulePath != "example.com/mod/sub" || rel != "pkg" {
		t.Fatalf("BestModuleForImport() = (%#v, %q, %v), want longest module match", module, rel, ok)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
}
