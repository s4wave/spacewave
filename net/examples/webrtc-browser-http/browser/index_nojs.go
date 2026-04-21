//go:build !js

package main

import "fmt"

func main() {
	fmt.Println("This program must be compiled with GOOS=js GOARCH=wasm")
}
