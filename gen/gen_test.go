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
			input.FSM.Transition = []string{tt.transition}
			_, err := ParseFSMData(input)
			if err == nil || !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("ParseFSMData() error = %v, want %q", err, tt.wantText)
			}
		})
	}
}

func TestLoadDataAcceptsSingleFSMObject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "door.fsm")
	content := []byte(`PackageDir: example.com/demo
Package: door
FSMEnginePackage: github.com/swkings
State: [Idle, Running]
Event: [Start]
FSM:
  Name: Door
  Describe: Door from YAML
  Initial: Idle
  Transition:
    - Idle -> Start -> Running
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	var input FSMYaml
	if err := LoadData(path, &input); err != nil {
		t.Fatal(err)
	}
	data, err := ParseFSMData(input)
	if err != nil {
		t.Fatal(err)
	}
	if data.FSM.Name != "Door" || data.FSM.Describe != "Door from YAML" ||
		data.FSM.InitialName != "Idle" || len(data.FSM.Transitions) != 1 {
		t.Fatalf("parsed FSM = %+v", data.FSM)
	}
}

func TestLegacyFSMsListIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.fsm")
	content := []byte(`PackageDir: example.com/demo
Package: door
FSMEnginePackage: github.com/swkings
State: [Idle, Running]
Event: [Start]
FSMs:
  - Name: Door
    Initial: Idle
    Transition:
      - Idle -> Start -> Running
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	var input FSMYaml
	err := LoadData(path, &input)
	if err == nil {
		_, err = ParseFSMData(input)
	}
	if err == nil {
		t.Fatal("legacy FSMs list was accepted")
	}
}

func TestParseFSMDataRequiresFSM(t *testing.T) {
	input := validFSMYAML()
	input.FSM = FSMMetaInfo{}
	if _, err := ParseFSMData(input); err == nil || !strings.Contains(err.Error(), "FSM.Name is required") {
		t.Fatalf("ParseFSMData() error = %v", err)
	}
}

func TestParseFSMDataRejectsDuplicateTransition(t *testing.T) {
	input := validFSMYAML()
	input.FSM.Transition = append(input.FSM.Transition, input.FSM.Transition[0])
	if _, err := ParseFSMData(input); err == nil || !strings.Contains(err.Error(), "duplicate transition") {
		t.Fatalf("ParseFSMData() error = %v", err)
	}
}

func TestParseFSMDataRejectsUnexportedEnumNames(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*FSMYaml)
	}{
		{
			name: "state",
			mutate: func(input *FSMYaml) {
				input.State[0] = "idle"
			},
		},
		{
			name: "event",
			mutate: func(input *FSMYaml) {
				input.Event[0] = "start"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validFSMYAML()
			tt.mutate(&input)
			if _, err := ParseFSMData(input); err == nil ||
				!strings.Contains(err.Error(), "exported Go identifiers") {
				t.Fatalf("ParseFSMData() error = %v", err)
			}
		})
	}
}

func TestGeneratorsProduceStableValidHandlerCode(t *testing.T) {
	data, err := ParseFSMData(validFSMYAML())
	if err != nil {
		t.Fatal(err)
	}
	outputDir := t.TempDir()
	templateDir := "template"

	if err := GenFSM(outputDir, "test.fsm", templateDir, data, true); err != nil {
		t.Fatal(err)
	}
	factory, err := os.ReadFile(filepath.Join(outputDir, data.Package, "fsmfactory.go"))
	if err != nil {
		t.Fatal(err)
	}
	factoryText := string(factory)
	if !strings.Contains(factoryText, `f.Describe = "Door workflow"`) {
		t.Fatalf("generated factory missing description:\n%s", factory)
	}
	if !strings.Contains(factoryText, "types.StateEnum.Idle") ||
		!strings.Contains(factoryText, "types.EventEnum.Start") ||
		!strings.Contains(factoryText, "handlers.GetHandler_s_Idle_e_Start()") {
		t.Fatalf("generated factory uses old handler naming:\n%s", factory)
	}
	if err := GenTypes(outputDir, "test.fsm", templateDir, data, true); err != nil {
		t.Fatal(err)
	}
	metadata, err := os.ReadFile(filepath.Join(outputDir, data.Package, "types", "fsmmetadata.go"))
	if err != nil {
		t.Fatal(err)
	}
	metadataText := string(metadata)
	if !strings.Contains(metadataText, "sIdle") ||
		!strings.Contains(metadataText, `fsm.FSMState("Idle")`) ||
		!strings.Contains(metadataText, "var StateEnum = stateEnumStruct{") ||
		!strings.Contains(metadataText, "eStart") ||
		!strings.Contains(metadataText, `fsm.FSMEvent("Start")`) ||
		!strings.Contains(metadataText, "var EventEnum = eventEnumStruct{") {
		t.Fatalf("generated metadata does not preserve YAML values:\n%s", metadata)
	}
	if err := GenHandlers(outputDir, "test.fsm", templateDir, data, true); err != nil {
		t.Fatal(err)
	}

	handlerPath := filepath.Join(outputDir, data.Package, "handlers", "s_Idle_e_Start.go")
	handlerCode, err := os.ReadFile(handlerPath)
	if err != nil {
		t.Fatal(err)
	}
	handlerText := string(handlerCode)
	if !strings.Contains(handlerText, "_Before_s_Idle_e_Start") ||
		!strings.Contains(handlerText, "_In_s_Idle_e_Start") ||
		!strings.Contains(handlerText, "_After_s_Idle_e_Start") {
		t.Fatalf("generated handler does not contain all stages:\n%s", handlerCode)
	}
	if got := strings.Count(handlerText, "f *fsm.FSM[types.Payload], _ ...any"); got != 3 {
		t.Fatalf("generated handler Actor signatures = %d, want 3:\n%s", got, handlerCode)
	}

	commonPath := filepath.Join(outputDir, data.Package, "handlers", "common.go")
	firstCommon, err := os.ReadFile(commonPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(firstCommon), "GetHandler_s_Idle_e_Start") {
		t.Fatalf("generated common handler uses old naming:\n%s", firstCommon)
	}
	if strings.Contains(string(firstCommon), "switch f.Name") || strings.Contains(string(firstCommon), "FSMName") || strings.Contains(handlerText, "ForDoor") {
		t.Fatalf("generated handlers still contain multi-FSM dispatch:\n%s\n%s", firstCommon, handlerCode)
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
		FSM: FSMMetaInfo{
			Name:       "Door",
			Describe:   "Door workflow",
			Initial:    "Idle",
			Transition: []string{"Idle -> Start -> Running", "Running -> Stop -> Idle"},
		},
	}
}
