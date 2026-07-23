package gen

import (
	"fmt"
	"path/filepath"
	"sort"
)

type CommonData struct {
	PackageDir       string
	Package          string
	FSMEnginePackage string
	HandleItemList   []*HandlerData
	FSMFileName      string
}

func GenCommons(outFile string, fsmFileName string, tplFileDir string, data FSMData) error {
	if err := CreateFSMDirLevel(filepath.Join(outFile, data.Package), "handlers"); err != nil {
		return err
	}

	handlerItems := collectHandlerData(data)
	items := make([]*HandlerData, 0, len(handlerItems))
	for _, item := range handlerItems {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		left, right := items[i].Transition, items[j].Transition
		if left.From == right.From {
			return left.Event < right.Event
		}
		return left.From < right.From
	})

	commonData := CommonData{
		PackageDir:       data.PackageDir,
		Package:          data.Package,
		FSMEnginePackage: data.FSMEnginePackage,
		HandleItemList:   items,
		FSMFileName:      fsmFileName,
	}

	outputPath := filepath.Join(outFile, data.Package, "handlers", "common.go")
	fmt.Printf("gen commonFile: %v\n", outputPath)
	return renderTemplateFile(outputPath, filepath.Join(tplFileDir, "common.tpl"), commonData)
}
