package gen

import (
	"fmt"
	"path/filepath"
	"sort"
)

func GenHandlers(outFile string, fsmFileName string, tplFileDir string, data FSMData, override bool) error {
	if err := CreateFSMDirLevel(filepath.Join(outFile, data.Package), "handlers"); err != nil {
		return err
	}

	for _, item := range collectHandlerData(data) {
		outputPath := filepath.Join(outFile, data.Package, "handlers", handlerFileName(item.Transition))
		exists, err := fileExists(outputPath)
		if err != nil {
			return err
		}
		if exists && !override {
			continue
		}

		fmt.Printf("gen handlerFile: %v\n", outputPath)
		if err := renderTemplateFile(outputPath, filepath.Join(tplFileDir, "handler.tpl"), item); err != nil {
			return err
		}
	}

	return GenCommons(outFile, fsmFileName, tplFileDir, data)
}

func collectHandlerData(data FSMData) []*HandlerData {
	items := make([]*HandlerData, 0, len(data.FSM.Transitions))
	for _, transition := range data.FSM.Transitions {
		items = append(items, &HandlerData{
			PackageDir:       data.PackageDir,
			Package:          data.Package,
			FSMEnginePackage: data.FSMEnginePackage,
			Transition:       transition,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		left, right := items[i].Transition, items[j].Transition
		if left.From == right.From {
			return left.Event < right.Event
		}

		return left.From < right.From
	})

	return items
}

func handlerFileName(transition Transition) string {
	return "s_" + transition.FromName + "_e_" + transition.EventName + ".go"
}
