package main

import (
	"log"
	"os"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/storage/memory"

	billy_memfs "github.com/go-git/go-billy/v6/memfs"
)

func main() {
	url := "."
	if len(os.Args) > 1 {
		url = os.Args[1]
	}

	log.Printf("cloning %s (recursive)...", url)

	fs := billy_memfs.New()
	storer := memory.NewStorage()

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
