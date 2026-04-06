package encoders_test

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
	"testing"

	encoders "github.com/thetechpanda/interfacify/pkg/encoders"
)

func TestCommentHelpers(t *testing.T) {
	t.Parallel()

	if got := encoders.CommentText(nil); got != "" {
		t.Fatalf("CommentText(nil) = %q, want empty string", got)
	}

	group := &ast.CommentGroup{List: []*ast.Comment{{Text: "// line one"}, {Text: "// line two"}}}
	if got := encoders.CommentText(group); got != "line one\nline two" {
		t.Fatalf("CommentText() = %q, want %q", got, "line one\nline two")
	}

	var comments strings.Builder
	encoders.WriteCommentLines(&comments, "line one\n\nline two", "")
	if got := comments.String(); got != "// line one\n//\n// line two\n" {
		t.Fatalf("WriteCommentLines() = %q, want blank comment line preserved", got)
	}
}

func TestTypeQualificationHelpers(t *testing.T) {
	t.Parallel()

	localTypes := map[string]*ast.TypeSpec{
		"Local":    {Name: ast.NewIdent("Local")},
		"Result":   {Name: ast.NewIdent("Result")},
		"Embedded": {Name: ast.NewIdent("Embedded")},
		"local":    {Name: ast.NewIdent("local")},
	}

	if encoders.QualifyLocalTypeRefs(nil, localTypes, "service") != nil {
		t.Fatal("QualifyLocalTypeRefs(nil) != nil")
	}
	if encoders.QualifyFieldListLocalTypeRefs(nil, localTypes, "service") != nil {
		t.Fatal("QualifyFieldListLocalTypeRefs(nil) != nil")
	}
	if encoders.QualifyFieldLocalTypeRefs(nil, localTypes, "service") != nil {
		t.Fatal("QualifyFieldLocalTypeRefs(nil) != nil")
	}
	if encoders.CloneBasicLit(nil) != nil {
		t.Fatal("CloneBasicLit(nil) != nil")
	}

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "ident", input: "Local", want: "service.Local"},
		{name: "array and binary", input: "[1+2]Local", want: "[1 + 2]service.Local"},
		{name: "map and star", input: "map[Local]*Local", want: "map[service.Local]*service.Local"},
		{name: "func and ellipsis", input: "func(Local, ...Local) (Result[Local], error)", want: "func(service.Local, ...service.Local) (service.Result[service.Local], error)"},
		{name: "index list", input: "Pair[Local, Result[Local]]", want: "Pair[service.Local, service.Result[service.Local]]"},
		{name: "selector preserved", input: "dep.Remote", want: "dep.Remote"},
		{name: "paren", input: "(Local)", want: "(service.Local)"},
		{name: "channel", input: "<-chan Local", want: "<-chan service.Local"},
		{name: "unary", input: "~Local", want: "~service.Local"},
	}

	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := printExpr(t, encoders.QualifyLocalTypeRefs(mustParseExpr(t, test.input), localTypes, "service"))
			if got != test.want {
				t.Fatalf("QualifyLocalTypeRefs(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}

	interfaceExpr := printExpr(t, encoders.QualifyLocalTypeRefs(mustParseExpr(t, "interface{ M(Local); Embedded }"), localTypes, "service"))
	if !strings.Contains(interfaceExpr, "M(service.Local)") || !strings.Contains(interfaceExpr, "service.Embedded") {
		t.Fatalf("QualifyLocalTypeRefs(interface) = %q, want qualified method and embedded type", interfaceExpr)
	}

	originalStruct := mustParseExpr(t, "struct{ Field Local `json:\"field\"`; Embedded }").(*ast.StructType)
	qualifiedStruct := encoders.QualifyLocalTypeRefs(originalStruct, localTypes, "service").(*ast.StructType)
	structText := printExpr(t, qualifiedStruct)
	if !strings.Contains(structText, "Field") || !strings.Contains(structText, "service.Local") || !strings.Contains(structText, "`json:\"field\"`") || !strings.Contains(structText, "service.Embedded") {
		t.Fatalf("QualifyLocalTypeRefs(struct) = %q, want qualified field, tag, and embedded type", structText)
	}
	if qualifiedStruct.Fields.List[0].Tag == nil || qualifiedStruct.Fields.List[0].Tag == originalStruct.Fields.List[0].Tag {
		t.Fatal("QualifyLocalTypeRefs(struct) did not clone field tag")
	}
}

func TestTypeUsageHelpers(t *testing.T) {
	t.Parallel()

	localTypes := map[string]*ast.TypeSpec{
		"Local":  {Name: ast.NewIdent("Local")},
		"Result": {Name: ast.NewIdent("Result")},
		"local":  {Name: ast.NewIdent("local")},
	}

	if !encoders.ExprUsesLocalTypes(mustParseExpr(t, "Local"), localTypes) {
		t.Fatal("ExprUsesLocalTypes(Local) = false, want true")
	}
	if encoders.ExprUsesLocalTypes(mustParseExpr(t, "dep.Remote"), localTypes) {
		t.Fatal("ExprUsesLocalTypes(dep.Remote) = true, want false")
	}
	if !encoders.ExprUsesLocalTypes(mustParseExpr(t, "Result[local]"), localTypes) {
		t.Fatal("ExprUsesLocalTypes(Result[local]) = false, want true")
	}
	if !encoders.ExprUsesLocalTypes(mustParseExpr(t, "func(dep.Remote, ...Local) interface{ M(local) }"), localTypes) {
		t.Fatal("ExprUsesLocalTypes(func...) = false, want true")
	}
	if encoders.FieldListUsesLocalTypes(nil, localTypes) {
		t.Fatal("FieldListUsesLocalTypes(nil) = true, want false")
	}

	funcType := mustParseExpr(t, "func(dep.Remote, ...Local) (Result[local], error)").(*ast.FuncType)
	if !encoders.FieldListUsesLocalTypes(funcType.Params, localTypes) {
		t.Fatal("FieldListUsesLocalTypes(params) = false, want true")
	}
	if !encoders.FieldListUsesLocalTypes(funcType.Results, localTypes) {
		t.Fatal("FieldListUsesLocalTypes(results) = false, want true")
	}

	if typeName, ok := encoders.FirstUnexportedLocalType(mustParseExpr(t, "map[dep.Remote]*local"), localTypes); !ok || typeName != "local" {
		t.Fatalf("FirstUnexportedLocalType(map...) = (%q, %v), want (%q, %v)", typeName, ok, "local", true)
	}
	if _, ok := encoders.FirstUnexportedLocalType(mustParseExpr(t, "Result[Local]"), localTypes); ok {
		t.Fatal("FirstUnexportedLocalType(Result[Local]) reported unexported type unexpectedly")
	}
	if _, ok := encoders.FirstUnexportedLocalType(nil, localTypes); ok {
		t.Fatal("FirstUnexportedLocalType(nil) reported unexported type unexpectedly")
	}
	if _, ok := encoders.FirstUnexportedLocalTypeInFieldList(nil, localTypes); ok {
		t.Fatal("FirstUnexportedLocalTypeInFieldList(nil) reported unexported type unexpectedly")
	}
	if typeName, ok := encoders.FirstUnexportedLocalTypeInFieldList(funcType.Results, localTypes); !ok || typeName != "local" {
		t.Fatalf("FirstUnexportedLocalTypeInFieldList(results) = (%q, %v), want (%q, %v)", typeName, ok, "local", true)
	}
}

func mustParseExpr(t *testing.T, src string) ast.Expr {
	t.Helper()

	expr, err := parser.ParseExpr(src)
	if err != nil {
		t.Fatalf("parser.ParseExpr(%q) error = %v", src, err)
	}

	return expr
}

func printExpr(t *testing.T, expr ast.Expr) string {
	t.Helper()

	var output strings.Builder
	if err := printer.Fprint(&output, token.NewFileSet(), expr); err != nil {
		t.Fatalf("printer.Fprint() error = %v", err)
	}

	return output.String()
}
