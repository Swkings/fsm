package gen

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"text/template"
)

func GenFSM(outFile string, fsmFileName string, tplFileDir string, data FSMData, _ bool) error {
	if err := CreateFSMDirLevel(outFile, data.Package); err != nil {
		return err
	}

	outputPath := filepath.Join(outFile, data.Package, "fsmfactory.go")
	fmt.Printf("gen fsmFactoryFile: %v\n", outputPath)

	return renderTemplateFile(outputPath, filepath.Join(tplFileDir, "fsm.tpl"), TemplateData{
		FSMFileName: fsmFileName,
		FSMData:     data,
	})
}

func renderTemplateFile(outputPath string, templatePath string, data any) error {
	tpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return fmt.Errorf("parse template %q: %w", templatePath, err)
	}

	var buffer bytes.Buffer
	if err := tpl.Execute(&buffer, data); err != nil {
		return fmt.Errorf("render generated file %q: %w", outputPath, err)
	}
	content := buffer.Bytes()
	if filepath.Ext(outputPath) == ".go" {
		content, err = format.Source(content)
		if err != nil {
			return fmt.Errorf("format generated file %q: %w", outputPath, err)
		}
	}

	if err := os.WriteFile(outputPath, content, 0o644); err != nil {
		return fmt.Errorf("write generated file %q: %w", outputPath, err)
	}

	return nil
}
