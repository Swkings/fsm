package gen

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFSMDataValidatesTransitionInput(t *testing.T) {
	tests := []struct {
		name       string
		transition string
		wantText   string
	}{
		{name: "malformed", transition: "Idle -> Start", wantText: "From -> Event -> To"},
		{name: "unknown state", transition: "Missing -> Start -> Running", wantText: "unknown source state"},
		{name: "unknown event", transition: "Idle -> Missing -> Running", wantText: "unknown event"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validFSMYAML()
			input.FSMs[0].Transition = []string{tt.transition}
			_, err := ParseFSMData(input)
			if err == nil || !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("ParseFSMData() error = %v, want %q", err, tt.wantText)
			}
		})
	}
}

func TestParseFSMDataRejectsDuplicateTransition(t *testing.T) {
	input := validFSMYAML()
	input.FSMs[0].Transition = append(input.FSMs[0].Transition, input.FSMs[0].Transition[0])
	if _, err := ParseFSMData(input); err == nil || !strings.Contains(err.Error(), "duplicate transition") {
		t.Fatalf("ParseFSMData() error = %v", err)
	}
}

func TestGeneratorsProduceStableValidMultiHandlerCode(t *testing.T) {
	data, err := ParseFSMData(validFSMYAML())
	if err != nil {
		t.Fatal(err)
	}
	outputDir := t.TempDir()
	templateDir := "template"

	if err := GenFSM(outputDir, "test.fsm", templateDir, data, true); err != nil {
		t.Fatal(err)
	}
	if err := GenTypes(outputDir, "test.fsm", templateDir, data, true); err != nil {
		t.Fatal(err)
	}
	if err := GenHandlers(outputDir, "test.fsm", templateDir, data, true); err != nil {
		t.Fatal(err)
	}

	handlerPath := filepath.Join(outputDir, data.Package, "handlers", "EventStart_StateIdle.go")
	handlerCode, err := os.ReadFile(handlerPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(handlerCode), "_BeforeHandlerForDoor") || !strings.Contains(string(handlerCode), "_AfterHandlerForDoor") {
		t.Fatalf("generated handler does not contain all stages:\n%s", handlerCode)
	}

	commonPath := filepath.Join(outputDir, data.Package, "handlers", "common.go")
	firstCommon, err := os.ReadFile(commonPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := GenHandlers(outputDir, "test.fsm", templateDir, data, false); err != nil {
		t.Fatal(err)
	}
	secondCommon, err := os.ReadFile(commonPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstCommon) != string(secondCommon) {
		t.Fatal("common handler output is not stable")
	}

	assertGeneratedGoParses(t, filepath.Join(outputDir, data.Package))
}

func TestGeneratedFilesAreTruncatedAndEditableFilesArePreserved(t *testing.T) {
	data, err := ParseFSMData(validFSMYAML())
	if err != nil {
		t.Fatal(err)
	}
	outputDir := t.TempDir()
	if err := CreateFSMDirLevel(outputDir, data.Package); err != nil {
		t.Fatal(err)
	}

	factoryPath := filepath.Join(outputDir, data.Package, "fsmfactory.go")
	if err := os.WriteFile(factoryPath, []byte(strings.Repeat("stale", 1000)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := GenFSM(outputDir, "test.fsm", "template", data, false); err != nil {
		t.Fatal(err)
	}
	factoryCode, err := os.ReadFile(factoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(factoryCode), "stale") {
		t.Fatal("generated factory retained stale file content")
	}

	if err := GenTypes(outputDir, "test.fsm", "template", data, true); err != nil {
		t.Fatal(err)
	}
	payloadPath := filepath.Join(outputDir, data.Package, "types", "payload.go")
	customPayload := []byte("package types\n\ntype Payload struct { Custom string }\n")
	if err := os.WriteFile(payloadPath, customPayload, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := GenTypes(outputDir, "test.fsm", "template", data, false); err != nil {
		t.Fatal(err)
	}
	preserved, err := os.ReadFile(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(preserved) != string(customPayload) {
		t.Fatal("payload was overwritten without override")
	}
}

func assertGeneratedGoParses(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		_, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.AllErrors)
		return err
	})
	if err != nil {
		t.Fatalf("generated Go does not parse: %v", err)
	}
}

func TestRenderTemplateDoesNotOverwriteOnInvalidGo(t *testing.T) {
	directory := t.TempDir()
	outputPath := filepath.Join(directory, "generated.go")
	templatePath := filepath.Join(directory, "invalid.tpl")
	original := []byte("package original\n")
	if err := os.WriteFile(outputPath, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(templatePath, []byte("package broken\nfunc"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := renderTemplateFile(outputPath, templatePath, nil); err == nil {
		t.Fatal("renderTemplateFile() error = nil")
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != string(original) {
		t.Fatalf("invalid generation overwrote original file: %q", content)
	}
}

func validFSMYAML() FSMYaml {
	return FSMYaml{
		PackageDir:       "example.com/demo",
		Package:          "door",
		FSMEnginePackage: "github.com/swkings",
		State:            []string{"Idle", "Running"},
		Event:            []string{"Start", "Stop"},
		FSMs: []FSMMetaInfo{
			{
				Name:       "Door",
				Initial:    "Idle",
				Transition: []string{"Idle -> Start -> Running", "Running -> Stop -> Idle"},
			},
		},
	}
}
