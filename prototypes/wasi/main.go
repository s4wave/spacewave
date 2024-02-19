package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	os.Stdout.WriteString("Hello wasi!\n")

	os.Stdout.WriteString("Listing all files...\n")

	walkPath := func(root string) {
		err := filepath.WalkDir(root, func(path string, ent os.DirEntry, err error) error {
			if err != nil {
				fmt.Printf("err accessing path %q: %v\n", path, err)
			} else {
				fmt.Printf("%s: %s\n", ent.Type().String(), path)
			}
			return nil
		})
		if err != nil {
			fmt.Printf("error walking the path %q: %v\n", "/", err)
		}
	}

	// each path is a separate file descriptor, so we have to address them directly.
	// walkPath("/")
	walkPath(".")
	walkPath("/tmp")
}
