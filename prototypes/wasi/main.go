package main

import "os"

func main() {
	os.Stdout.WriteString("Hello wasi!\n")
}
