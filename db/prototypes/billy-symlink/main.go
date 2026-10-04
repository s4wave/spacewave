//go:build !js && !wasip1

package main

import (
	"fmt"
	ifs "io/fs"
	"os"
	"path"

	// billy "github.com/go-git/go-billy/v6"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/go-git/go-billy/v6/osfs"
)

func main() {
	// Create a filesystem view rooted at the current directory.
	fs := osfs.New("./", osfs.WithBoundOS())

	// Prepare the root directory and remove any previous example output.
	rootDir := "mydir"
	if err := fsutil.CleanDir("./" + rootDir); err != nil {
		fmt.Printf("Error removing target directory: %v\n", err)
		return
	}

	// Create the example root directory.
	err := fs.MkdirAll(rootDir, os.ModePerm)
	if err != nil {
		fmt.Printf("Error creating root directory: %v\n", err)
		return
	}

	// Create a child directory to serve as the symlink target.
	targetDir := path.Join(rootDir, "target")
	err = fs.MkdirAll(targetDir, os.ModePerm)
	if err != nil {
		fmt.Printf("Error creating target directory: %v\n", err)
		return
	}

	// Create a symbolic link from src to the target directory.
	srcLink := path.Join(rootDir, "src")
	err = fs.Symlink("./target", srcLink)
	if err != nil {
		fmt.Printf("Error creating symbolic link: %v\n", err)
		return
	}

	// Report that the directory and symbolic link are ready.
	fmt.Println("Directory and symlink creation successful.")

	// Read the root directory entries for inspection.
	fi, err := fs.ReadDir(rootDir)
	if err != nil {
		fmt.Printf("Error calling readdir: %v\n", err)
		return
	}

	// Print each entry's mode and symlink status.
	for _, entry := range fi {
		info, err := entry.Info()
		if err != nil {
			fmt.Printf("Error getting info for %v: %v\n", entry.Name(), err)
			continue
		}
		fmt.Printf(
			"Entry: %v %v -> symlink(%v) dir(%v)\n",
			entry.Name(),
			info.Mode().String(),
			info.Mode().Type()&ifs.ModeSymlink != 0,
			entry.IsDir(),
		)
	}
}
