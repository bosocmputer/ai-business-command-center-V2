package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"
)

// notWiredOnPurpose lists fields of httpapi.Dependencies that main may leave unset, each with the reason. It is empty: every service the
// handler can serve must be given to it, because a field left out makes its routes answer 404 while every test of the service still passes
// (this happened to the assistant settings routes on 9 Oct 2026).
var notWiredOnPurpose = map[string]string{}

func parse(t *testing.T, path string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

// dependencyFields are the fields of the struct that NewHandler is given.
func dependencyFields(t *testing.T) []string {
	t.Helper()
	var fields []string
	ast.Inspect(parse(t, "../../internal/httpapi/handler.go"), func(node ast.Node) bool {
		spec, ok := node.(*ast.TypeSpec)
		if !ok || spec.Name.Name != "Dependencies" {
			return true
		}
		for _, field := range spec.Type.(*ast.StructType).Fields.List {
			for _, name := range field.Names {
				fields = append(fields, name.Name)
			}
		}
		return false
	})
	if len(fields) == 0 {
		t.Fatal("could not find httpapi.Dependencies")
	}
	return fields
}

// wiredKeys are the fields main.go gives to httpapi.Dependencies, with the text of what it gives.
func wiredKeys(t *testing.T) map[string]string {
	t.Helper()
	keys := map[string]string{}
	found := false
	ast.Inspect(parse(t, "main.go"), func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		selector, ok := literal.Type.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Dependencies" {
			return true
		}
		found = true
		for _, element := range literal.Elts {
			pair, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			value := ""
			if ident, ok := pair.Value.(*ast.Ident); ok {
				value = ident.Name
			}
			keys[pair.Key.(*ast.Ident).Name] = value
		}
		return false
	})
	if !found {
		t.Fatal("could not find the httpapi.Dependencies literal in main.go")
	}
	return keys
}

func TestMainGivesTheHandlerEveryServiceItCanServe(t *testing.T) {
	keys := wiredKeys(t)
	var missing []string
	for _, field := range dependencyFields(t) {
		if _, wired := keys[field]; !wired {
			if _, reason := notWiredOnPurpose[field]; !reason {
				missing = append(missing, field)
			}
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("main.go does not set these httpapi.Dependencies fields, so their routes answer 404: %s", strings.Join(missing, ", "))
	}
}

func TestMainNeverHandsTheHandlerAnExplicitNil(t *testing.T) {
	for field, value := range wiredKeys(t) {
		if value == "nil" {
			t.Errorf("main.go sets %s to nil", field)
		}
	}
}

func TestEveryFieldLeftOutOnPurposeStillExists(t *testing.T) {
	declared := map[string]bool{}
	for _, field := range dependencyFields(t) {
		declared[field] = true
	}
	for field := range notWiredOnPurpose {
		if !declared[field] {
			t.Errorf("%s is listed as not wired on purpose but is not a field any more", field)
		}
	}
}
