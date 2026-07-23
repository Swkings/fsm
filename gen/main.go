//go:build ignore

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/swkings/fsm/gen"
)

var (
	packageFatherDir = flag.String("out", "", "the package father dir")
	templateDir      = flag.String("tpl", "", "the template file dir")
	fsmDescribeFile  = flag.String("fsm", "", "the fsm describe file")
	override         = flag.Bool("c", false, "override user-editable payload and handler files")
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	flag.Parse()
	if strings.TrimSpace(*packageFatherDir) == "" {
		workingDirectory, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
		*packageFatherDir = workingDirectory
	}

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("locate generator directory")
	}
	generatorDir := filepath.Dir(currentFile)
	if strings.TrimSpace(*templateDir) == "" {
		*templateDir = filepath.Join(generatorDir, "template")
	}
	if strings.TrimSpace(*fsmDescribeFile) == "" {
		*fsmDescribeFile = filepath.Join(generatorDir, "door.fsm")
	}

	fmt.Printf("packageFatherDir: %v\n", *packageFatherDir)
	fmt.Printf("templateDir: %v\n", *templateDir)
	fmt.Printf("fsmDescribeFile: %v\n", *fsmDescribeFile)

	var fsmYAML gen.FSMYaml
	if err := gen.LoadData(*fsmDescribeFile, &fsmYAML); err != nil {
		return err
	}
	data, err := gen.ParseFSMData(fsmYAML)
	if err != nil {
		return err
	}

	fsmFileName := filepath.Base(*fsmDescribeFile)
	if err := gen.GenFSM(*packageFatherDir, fsmFileName, *templateDir, data, *override); err != nil {
		return err
	}
	if err := gen.GenTypes(*packageFatherDir, fsmFileName, *templateDir, data, *override); err != nil {
		return err
	}
	if err := gen.GenHandlers(*packageFatherDir, fsmFileName, *templateDir, data, *override); err != nil {
		return err
	}
	return nil
}
