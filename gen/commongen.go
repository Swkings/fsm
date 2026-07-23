package gen

import (
	"fmt"
	"log"
	"os"

	"text/template"

	"github.com/swkings/fsm"
)

type CommonData struct {
	PackageDir       string
	Package          string
	FSMEnginePackage string
	HandleItemList   []*HandlerData
	FSMFileName      string
}

func GenCommons(outFile string, fsmFileName string, tplFileDir string, data FSMData) {
	err := CreateFSMDirLevel(outFile+"/"+data.Package, "/handlers")
	if err != nil {
		log.Fatal(err)
	}

	var handleItemMap = map[fsm.EventStateKey]*HandlerData{}
	for _, fsmData := range data.FSMs {
		for _, transition := range fsmData.Transitions {
			eventStateKey := fsm.EventStateKey{
				Event:     fsm.FSMEvent(transition.Event),
				FromState: fsm.FSMState(transition.From),
			}
			if _, ok := handleItemMap[eventStateKey]; !ok {
				handleItemMap[eventStateKey] = &HandlerData{
					PackageDir:       data.PackageDir,
					Package:          data.Package,
					FSMEnginePackage: data.FSMEnginePackage,
					Transition:       transition,
					FSMNameList:      []fsm.FSMName{},
				}
			}
			handleItemMap[eventStateKey].FSMNameList = append(handleItemMap[eventStateKey].FSMNameList, fsm.FSMName(fsmData.Name))
		}
	}

	var commonData CommonData = CommonData{
		PackageDir:       data.PackageDir,
		Package:          data.Package,
		FSMEnginePackage: data.FSMEnginePackage,
		HandleItemList: Map2ListOpt(handleItemMap, func(_ fsm.EventStateKey, hd *HandlerData) *HandlerData {
			return hd
		}),
		FSMFileName: fsmFileName,
	}

	handlersDir := fmt.Sprintf("%v/%v/handlers", outFile, data.Package)

	commonTpl, err := template.ParseFiles(tplFileDir + "/single_handler_common.tpl")
	if err != nil {
		log.Fatal(err)
	}

	commonFilePathName := handlersDir + "/common.go"

	fmt.Printf("gen commonFile: %v\n", commonFilePathName)
	payloadFile, _ := os.OpenFile(commonFilePathName, os.O_CREATE|os.O_RDWR, 0666)
	defer payloadFile.Close()
	err = commonTpl.Execute(payloadFile, commonData)
	if err != nil {
		log.Fatal(err)
	}

}
