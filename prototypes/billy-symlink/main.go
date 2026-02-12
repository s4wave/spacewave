//go:build !js && !wasip1

package main

import (
	"fmt"
	ifs "io/fs"
	"os"
	"path"

	// billy "github.com/go-git/go-billy/v5"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/go-git/go-billy/v5/osfs"
)

func main() {
	fs := osfs.New("./", osfs.WithChrootOS())

	// Create root directory
	rootDir := "mydir"
	if err := fsutil.CleanDir("./" + rootDir); err != nil {
		fmt.Printf("Error removing target directory: %v\n", err)
		return
	}

	err := fs.MkdirAll(rootDir, os.ModePerm)
	if err != nil {
		fmt.Printf("Error creating root directory: %v\n", err)
		return
	}

	// Create 'target' directory
	targetDir := path.Join(rootDir, "target")
	err = fs.MkdirAll(targetDir, os.ModePerm)
	if err != nil {
		fmt.Printf("Error creating target directory: %v\n", err)
		return
	}

	// Create symbolic link 'src' pointing to 'target'
	srcLink := path.Join(rootDir, "src")
	err = fs.Symlink("./target", srcLink)
	if err != nil {
		fmt.Printf("Error creating symbolic link: %v\n", err)
		return
	}

	fmt.Println("Directory and symlink creation successful.")

	fi, err := fs.ReadDir(rootDir)
	if err != nil {
		fmt.Printf("Error calling readdir: %v\n", err)
		return
	}

	for _, entry := range fi {
		fmt.Printf(
			"Entry: %v %v -> symlink(%v) dir(%v)\n",
			entry.Name(),
			entry.Mode().String(),
			entry.Mode().Type()&ifs.ModeSymlink != 0,
			entry.IsDir(),
		)
	}
}
