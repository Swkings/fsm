package gen

import (
	"fmt"
	"log"
	"os"
	"text/template"
)

func GenFSM(outFile string, fsmFileName string, tplFileDir string, data FSMData, override bool) {
	err := CreateFSMDirLevel(outFile, "/"+data.Package)
	if err != nil {
		log.Fatal(err)
	}

	fsmFactoryFilePathName := fmt.Sprintf("%v/%v/fsmfactory.go", outFile, data.Package)
	fmt.Printf("gen fsmFactoryFile: %v\n", fsmFactoryFilePathName)
	file, _ := os.OpenFile(fsmFactoryFilePathName, os.O_CREATE|os.O_RDWR, 0666)
	defer file.Close()

	// Define the template
	fsmTpl, err := template.ParseFiles(tplFileDir + "/fsm.tpl")
	if err != nil {
		log.Fatal(err)
	}
	// Execute the template
	err = fsmTpl.Execute(file, TemplateData{
		FSMFileName: fsmFileName,
		FSMData:     data,
	})
	if err != nil {
		log.Fatal(err)
	}
}
