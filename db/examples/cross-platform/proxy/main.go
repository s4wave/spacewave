package main

import (
	"net/http"
	"os"
	"path"

	"github.com/sosedoff/gitkit"
)

func run() error {
	// Configure git service
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	service := gitkit.New(gitkit.Config{
		Dir: path.Join(wd, "../../../"),
	})
	if err := service.Setup(); err != nil {
		return err
	}

	http.HandleFunc("/.git/", func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Access-Control-Allow-Origin", "*")
		rw.Header().Set("Access-Control-Allow-Headers", "*")
		if req.Method == "OPTIONS" {
			rw.WriteHeader(200)
			return
		}
		if req.URL.Path == "/.git/git-upload-pack" {
			// Bug: wasm sends GET for this URL
			req.Method = "POST"
		}
		service.ServeHTTP(rw, req)
	})
	http.HandleFunc("/wasm_exec.js", func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "text/javascript")
		http.ServeFile(rw, req, path.Join(wd, "../wasm_exec.js"))
	})
	http.HandleFunc("/test.wasm", func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/wasm")
		http.ServeFile(rw, req, path.Join(wd, "../test.wasm"))
	})
	http.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/":
		case "/index.html":
		case "/wasm_exec.html":
		default:
			return
		}
		http.ServeFile(rw, req, path.Join(wd, "../wasm_exec.html"))
	})

	// Start HTTP server
	os.Stderr.WriteString("listening on :5000\n")
	return http.ListenAndServe(":5000", nil)
}

func main() {
	if err := run(); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
