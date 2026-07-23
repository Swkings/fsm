package gen

import (
	"fmt"
	"log"
	"os"

	"text/template"

	"github.com/swkings/fsm"
)

type HandlerData struct {
	PackageDir       string
	Package          string
	FSMEnginePackage string
	Transition       Transition
	FSMNameList      []fsm.FSMName
}

func GenHandlers(outFile string, fsmFileName string, tplFileDir string, data FSMData, override bool) {
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

	handlersDir := fmt.Sprintf("%v/%v/handlers", outFile, data.Package)

	handlerTpl, err := template.ParseFiles(tplFileDir + "/single_handler.tpl")
	if err != nil {
		log.Fatal(err)
	}

	for key, handlerData := range handleItemMap {
		handlerFilePathName := handlersDir + "/" + key.String() + ".go"
		if isExist(handlerFilePathName) && !override {
			continue
		}
		fmt.Printf("gen handlerFile: %v\n", handlerFilePathName)
		payloadFile, _ := os.OpenFile(handlerFilePathName, os.O_CREATE|os.O_RDWR, 0666)
		defer payloadFile.Close()
		err = handlerTpl.Execute(payloadFile, handlerData)
		if err != nil {
			log.Fatal(err)
		}
	}

	GenCommons(outFile, fsmFileName, tplFileDir, data)
}
