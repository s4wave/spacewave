package main

import (
	"log"
	"os"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/storage/memory"

	billy_memfs "github.com/go-git/go-billy/v6/memfs"
)

func main() {
	// Resolve the requested repository URL.
	url := "."
	if len(os.Args) > 1 {
		url = os.Args[1]
	}

	// Prepare in-memory storage and a filesystem for the recursive clone.
	log.Printf("cloning %s (recursive)...", url)
	fs := billy_memfs.New()
	storer := memory.NewStorage()

	// Clone the repository and report the result.
	_, err := git.Clone(storer, fs, &git.CloneOptions{
		URL:               url,
		RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
		Progress:          os.Stderr,
	})
	if err != nil {
		log.Fatalf("clone failed: %v", err)
	}

	log.Printf("clone succeeded")
}
