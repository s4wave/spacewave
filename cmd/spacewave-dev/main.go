//go:build !js

package main

import (
	"fmt"
	"os"

	_ "github.com/aperturerobotics/controllerbus/example/boilerplate"
	"github.com/s4wave/spacewave/dev"

	_ "github.com/s4wave/spacewave/bldr/values"
)

func main() {
	if err := dev.BuildApp().Run(os.Args); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
