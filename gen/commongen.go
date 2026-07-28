package gen

import (
	"fmt"
	"path/filepath"
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

	commonData := CommonData{
		PackageDir:       data.PackageDir,
		Package:          data.Package,
		FSMEnginePackage: data.FSMEnginePackage,
		HandleItemList:   collectHandlerData(data),
		FSMFileName:      fsmFileName,
	}

	outputPath := filepath.Join(outFile, data.Package, "handlers", "common.go")
	fmt.Printf("gen commonFile: %v\n", outputPath)
	return renderTemplateFile(outputPath, filepath.Join(tplFileDir, "common.tpl"), commonData)
}
