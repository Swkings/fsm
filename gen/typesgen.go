package gen

import (
	"fmt"
	"log"
	"os"
	"text/template"
)

func GenTypes(outFile string, fsmFileName string, tplFileDir string, data FSMData, override bool) {
	err := CreateFSMDirLevel(outFile+"/"+data.Package, "/types")
	if err != nil {
		log.Fatal(err)
	}

	genPayload(outFile, fsmFileName, tplFileDir, data, override)
	genFsmMetaData(outFile, fsmFileName, tplFileDir, data, override)
}

func genPayload(outFile string, fsmFileName string, tplFileDir string, data FSMData, override bool) {
	typesDir := fmt.Sprintf("%v/%v/types", outFile, data.Package)
	payloadFilePathName := typesDir + "/payload.go"
	if isExist(payloadFilePathName) && !override {
		return
	}
	fmt.Printf("gen payloadFile: %v\n", payloadFilePathName)
	payloadFile, _ := os.OpenFile(payloadFilePathName, os.O_CREATE|os.O_RDWR, 0666)
	defer payloadFile.Close()

	payloadTpl, err := template.ParseFiles(tplFileDir + "/payload.tpl")
	if err != nil {
		log.Fatal(err)
	}
	err = payloadTpl.Execute(payloadFile, TemplateData{
		FSMFileName: fsmFileName,
		FSMData:     data,
	})
	if err != nil {
		log.Fatal(err)
	}
}

func genFsmMetaData(outFile string, fsmFileName string, tplFileDir string, data FSMData, override bool) {
	typesDir := fmt.Sprintf("%v/%v/types", outFile, data.Package)
	metaDataFilePathName := typesDir + "/fsmmetadata.go"
	fmt.Printf("gen metaDataFile: %v\n", metaDataFilePathName)
	payloadFile, _ := os.OpenFile(metaDataFilePathName, os.O_CREATE|os.O_RDWR, 0666)
	defer payloadFile.Close()

	metaDataTpl, err := template.ParseFiles(tplFileDir + "/fsmmetadata.tpl")
	if err != nil {
		log.Fatal(err)
	}
	err = metaDataTpl.Execute(payloadFile, TemplateData{
		FSMFileName: fsmFileName,
		FSMData:     data,
	})
	if err != nil {
		log.Fatal(err)
	}
}
