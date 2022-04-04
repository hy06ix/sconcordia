package main

import (
	// Service needs to be imported here to be instantiated.

	"github.com/hy06ix/onet/simul"
	_ "github.com/hy06ix/sconcordia/simulation"
)

func main() {
	simul.Start()
}
