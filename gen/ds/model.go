// Package ds contains the code generator's input and template data models.
package ds

// TemplateData is the root context shared by generated templates.
type TemplateData struct {
	FSMFileName string
	FSMData     FSMData
}

// FSMMetaInfo is the FSM section accepted from YAML.
type FSMMetaInfo struct {
	Name       string
	Describe   string
	Initial    string
	Transition []string
}

// FSMYaml is the complete generator YAML input.
type FSMYaml struct {
	PackageDir       string
	Package          string
	FSMEnginePackage string
	State            []string
	Event            []string
	FSM              FSMMetaInfo
}

// Symbol pairs a generated Go identifier with its configured value.
type Symbol struct {
	Identifier string
	Value      string
}

// Transition is one validated transition used by templates.
type Transition struct {
	From      string
	FromName  string
	To        string
	ToName    string
	Event     string
	EventName string
}

// FSM is the validated FSM template model.
type FSM struct {
	Name        string
	Describe    string
	Initial     string
	InitialName string
	Transitions []Transition
}

// FSMData is the complete validated template model.
type FSMData struct {
	PackageDir       string
	Package          string
	FSMEnginePackage string
	States           []Symbol
	Events           []Symbol
	FSM              FSM
}

// HandlerData is the context used to render one transition handler.
type HandlerData struct {
	PackageDir       string
	Package          string
	FSMEnginePackage string
	Transition       Transition
}

// CommonData is the context used to render shared handler glue.
type CommonData struct {
	PackageDir       string
	Package          string
	FSMEnginePackage string
	HandleItemList   []*HandlerData
	FSMFileName      string
}
