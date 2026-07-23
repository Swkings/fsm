package gen

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/swkings/fsm"
)

type HandlerData struct {
	PackageDir       string
	Package          string
	FSMEnginePackage string
	Transition       Transition
	FSMNameList      []fsm.FSMName
}

func GenHandlers(outFile string, fsmFileName string, tplFileDir string, data FSMData, override bool) error {
	if err := CreateFSMDirLevel(filepath.Join(outFile, data.Package), "handlers"); err != nil {
		return err
	}

	handlerItems := collectHandlerData(data)
	keys := make([]fsm.EventStateKey, 0, len(handlerItems))
	for key := range handlerItems {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].FromState == keys[j].FromState {
			return keys[i].Event < keys[j].Event
		}
		return keys[i].FromState < keys[j].FromState
	})

	for _, key := range keys {
		outputPath := filepath.Join(outFile, data.Package, "handlers", key.String()+".go")
		exists, err := fileExists(outputPath)
		if err != nil {
			return err
		}
		if exists && !override {
			continue
		}

		fmt.Printf("gen handlerFile: %v\n", outputPath)
		if err := renderTemplateFile(outputPath, filepath.Join(tplFileDir, "handler.tpl"), handlerItems[key]); err != nil {
			return err
		}
	}

	return GenCommons(outFile, fsmFileName, tplFileDir, data)
}

func collectHandlerData(data FSMData) map[fsm.EventStateKey]*HandlerData {
	items := make(map[fsm.EventStateKey]*HandlerData)
	for _, machine := range data.FSMs {
		for _, transition := range machine.Transitions {
			key := fsm.EventStateKey{
				Event:     fsm.FSMEvent(transition.Event),
				FromState: fsm.FSMState(transition.From),
			}
			if _, exists := items[key]; !exists {
				items[key] = &HandlerData{
					PackageDir:       data.PackageDir,
					Package:          data.Package,
					FSMEnginePackage: data.FSMEnginePackage,
					Transition:       transition,
				}
			}
			items[key].FSMNameList = append(items[key].FSMNameList, fsm.FSMName(machine.Name))
		}
	}
	return items
}
