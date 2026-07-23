//go:build ignore

package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"scheduler-subway/common/fsm/gen"
	"strings"
)

var (
	packageFatherDir = flag.String("out", "", "the package father dir")
	templateDir      = flag.String("tpl", "", "the template file dir")
	fsmDescribeFile  = flag.String("fsm", "", "the fsm describe file")
	override         = flag.Bool("c", false, "override all file")
)

func main() {
	flag.Parse()
	if strings.Trim(*packageFatherDir, " ") == "" {
		*packageFatherDir, _ = os.Getwd()
	}
	_, fullFilePath, _, _ := runtime.Caller(0)
	fullFilePathItems := strings.Split(fullFilePath, "/")
	mainFileDir := strings.Join(fullFilePathItems[:len(fullFilePathItems)-1], "/")
	if strings.Trim(*templateDir, " ") == "" {
		*templateDir = fmt.Sprintf("%v/template", mainFileDir)
	} else if (*templateDir)[len(*templateDir)-1] == '/' {
		*templateDir = (*templateDir)[:len(*templateDir)-1]
	}
	if strings.Trim(*fsmDescribeFile, " ") == "" {
		*fsmDescribeFile = fmt.Sprintf("%v/door.fsm", mainFileDir)
	}

	fmt.Printf("packageFatherDir: %v\n", *packageFatherDir)
	fmt.Printf("templateDir: %v\n", *templateDir)
	fmt.Printf("fsmDescribeFile: %v\n", *fsmDescribeFile)

	var fsmYaml gen.FSMYaml

	gen.LoadData(*fsmDescribeFile, &fsmYaml)

	// Define the FSM data
	data := gen.ParseFSMData(fsmYaml)
	// fmt.Printf("fsmYaml: %v\n", data)

	fsmFileName := func() string {
		fsmDescribeFileItems := strings.Split(*fsmDescribeFile, "/")
		return fsmDescribeFileItems[len(fsmDescribeFileItems)-1]
	}()

	gen.GenFSM(*packageFatherDir, fsmFileName, *templateDir, data, *override)
	gen.GenTypes(*packageFatherDir, fsmFileName, *templateDir, data, *override)
	gen.GenHandlers(*packageFatherDir, fsmFileName, *templateDir, data, *override)

}
