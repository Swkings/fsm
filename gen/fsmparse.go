package gen

import (
	"os"
	"strings"

	"github.com/zeromicro/go-zero/core/conf"
)

type TemplateData struct {
	FSMFileName string
	FSMData     FSMData
}

const (
	eventPrefix = "Event"
	statePrefix = "State"
)

var EventMap = map[string]string{}
var StateMap = map[string]string{}

type FSMMetaInfo struct {
	Name       string
	Describe   string `yaml:"Describe,omitempty"`
	Initial    string
	Transition []string
}

type FSMYaml struct {
	PackageDir       string
	Package          string
	FSMEnginePackage string
	State            []string
	Event            []string
	FSMs             []FSMMetaInfo
}

type Transition struct {
	From  string
	To    string
	Event string
}
type FSM struct {
	Name        string
	Describe    string
	Initial     string
	Transitions []Transition
}
type FSMData struct {
	PackageDir       string
	Package          string
	FSMEnginePackage string
	States           []string
	Events           []string
	FSMs             []FSM
}

func ParseFSMData(fsmYaml FSMYaml) FSMData {
	fsms := []FSM{}
	for _, fsmsMetaInfo := range fsmYaml.FSMs {
		transitions := []Transition{}
		for _, transition := range fsmsMetaInfo.Transition {
			itemList := strings.Split(transition, "->")
			transitions = append(transitions, Transition{
				From:  statePrefix + strings.TrimSpace(itemList[0]),
				Event: eventPrefix + strings.TrimSpace(itemList[1]),
				To:    statePrefix + strings.TrimSpace(itemList[2]),
			})
		}
		fsms = append(fsms, FSM{
			Name:        fsmsMetaInfo.Name,
			Describe:    fsmsMetaInfo.Describe,
			Initial:     statePrefix + fsmsMetaInfo.Initial,
			Transitions: transitions,
		})
	}
	return FSMData{
		PackageDir:       fsmYaml.PackageDir,
		Package:          fsmYaml.Package,
		FSMEnginePackage: fsmYaml.FSMEnginePackage,
		States: append([]string{}, func(states []string) []string {
			res := []string{}
			for _, item := range states {
				res = append(res, statePrefix+item)
			}
			return res
		}(fsmYaml.State)...),
		Events: append([]string{}, func(events []string) []string {
			res := []string{}
			for _, item := range events {
				res = append(res, eventPrefix+item)
			}
			return res
		}(fsmYaml.Event)...),
		FSMs: fsms,
	}
}

func LoadData(file string, v interface{}) error {
	content, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	return conf.LoadFromYamlBytes(content, v)
}

func isExist(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		return os.IsExist(err)
	}

	return true
}

func CreateDir(path string) error {
	if !isExist(path) {
		err := os.MkdirAll(path, os.ModePerm)
		if err != nil {
			return err
		}
		return err
	}

	return nil
}

func CreateFSMDirLevel(path string, childLevels ...string) error {
	// create top dir
	err := CreateDir(path)

	if err != nil {
		return err
	}

	for _, childLevel := range childLevels {
		err = CreateDir(path + childLevel)
		if err != nil {
			return err
		}
	}

	return nil
}

// map[K]V to List[GetValue(K,V)]
func Map2ListOpt[T comparable, V any, R any](m map[T]V, GetValue func(T, V) R, filter ...func(key T, item V) bool) []R {
	res := []R{}
	for key, item := range m {
		if len(filter) > 0 && filter[0](key, item) {
			continue
		}
		res = append(res, GetValue(key, item))
	}

	return res
}
