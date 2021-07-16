package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/evanw/esbuild/pkg/api"
)

func main() {
	tsx, err := ioutil.ReadFile("src/index.tsx")
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	result := api.Transform(string(tsx), api.TransformOptions{
		Loader: api.LoaderTSX,
	})

	fmt.Printf("%d errors and %d warnings\n",
		len(result.Errors), len(result.Warnings))

	os.Stdout.Write(result.Code)
}
