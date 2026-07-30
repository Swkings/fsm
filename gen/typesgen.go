package gen

import (
	"fmt"
	"path/filepath"
)

func GenTypes(outFile string, fsmFileName string, tplFileDir string, data FSMData, override bool) error {
	if err := CreateFSMDirLevel(filepath.Join(outFile, data.Package), "types"); err != nil {
		return err
	}
	if err := genPayload(outFile, fsmFileName, tplFileDir, data, override); err != nil {
		return err
	}

	return genFSMMetaData(outFile, fsmFileName, tplFileDir, data)
}

func genPayload(outFile string, fsmFileName string, tplFileDir string, data FSMData, override bool) error {
	outputPath := filepath.Join(outFile, data.Package, "types", "payload.go")
	exists, err := fileExists(outputPath)
	if err != nil {
		return err
	}
	if exists && !override {
		return nil
	}

	fmt.Printf("gen payloadFile: %v\n", outputPath)

	return renderTemplateFile(outputPath, filepath.Join(tplFileDir, "payload.tpl"), TemplateData{
		FSMFileName: fsmFileName,
		FSMData:     data,
	})
}

func genFSMMetaData(outFile string, fsmFileName string, tplFileDir string, data FSMData) error {
	outputPath := filepath.Join(outFile, data.Package, "types", "fsmmetadata.go")
	fmt.Printf("gen metaDataFile: %v\n", outputPath)

	return renderTemplateFile(outputPath, filepath.Join(tplFileDir, "fsmmetadata.tpl"), TemplateData{
		FSMFileName: fsmFileName,
		FSMData:     data,
	})
}
