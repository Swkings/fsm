package util

import (
	"fmt"
	"testing"
)

type C struct {
	BMap *B
	Id   string
	Name string
	Age  int32
}

type A struct {
	AA1 string
	AA2 int32
}

type B struct {
	AAA A
	BB1 string
	BB2 string
}

func TestPrettyMapStruct(t *testing.T) {
	c := &C{
		BMap: &B{
			AAA: A{
				AA1: "AA1",
				AA2: 222,
			},
			BB1: "BB1",
			BB2: "BB2",
		},
		Id:   "000",
		Name: "Test",
		Age:  100,
	}
	fmt.Printf("%+v\n", c)
	fmt.Println("============")
	pcIndent := PrettyMapStruct(c, true)
	fmt.Println(pcIndent)
	fmt.Printf("Data1: %+v", pcIndent)
	fmt.Println("============")
	pc := PrettyMapStruct(c, false)
	fmt.Println(pc)
	fmt.Printf("Data2: %+v", pc)
}
