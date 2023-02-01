package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"unicode/utf8"
)

// DelveAddr is the address to listen with headless delve.
// If empty, uses "go build" then runs the binary.
var DelveAddr string

// BuildFlags is the list of build flags.
// Can be overridden by the compiler.
var BuildFlags []string

// BuildEnv is the list of build environment variables.
// Can be overridden by the compiler.
var BuildEnv []string

func main() {
	err := run()
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func run() error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	srcDir := wd
	runCmd := func(entry string, withStdio bool, args ...string) error {
		ecmd := exec.Command(entry, args...)
		ecmd.Env = append(os.Environ(), BuildEnv...)
		ecmd.Dir = srcDir
		if withStdio {
			ecmd.Stdin = os.Stdin
			ecmd.Stdout = os.Stdout
		} else {
			ecmd.Stdout = os.Stderr
		}
		ecmd.Stderr = os.Stderr
		os.Stderr.WriteString(ecmd.String() + "\n")
		if err := ecmd.Start(); err != nil {
			return err
		}
		subCtx, subCtxCancel := context.WithCancel(context.Background())
		defer subCtxCancel()
		go func() {
			// forward sigint to subprocess
			ch := make(chan os.Signal, 1)
			signal.Notify(ch, os.Interrupt)
			for {
				select {
				case <-subCtx.Done():
					return
				case sig := <-ch:
					_ = ecmd.Process.Signal(sig)
				}
			}
		}()
		defer func() {
			_ = ecmd.Process.Kill()
		}()
		return ecmd.Wait()
	}

	if DelveAddr == "wait" {
		if os.Args[len(os.Args)-1] == "exec-plugin" {
			os.Stderr.WriteString("Waiting for you to manually run the plugin entrypoint.\n")
			ch := make(chan os.Signal, 1)
			signal.Notify(ch, os.Interrupt)
			<-ch
			return nil
		}

		// run interactively
		return runCmd(
			"dlv", true,
			"debug",
			"--build-flags", shellQuoteJoin(BuildFlags...),
			"--",
			"exec-plugin",
		)
	}

	if DelveAddr != "" {
		return runCmd(
			"dlv", true,
			"debug",
			"--listen",
			DelveAddr,
			"--headless",
			"--accept-multiclient",
			"--build-flags", shellQuoteJoin(BuildFlags...),
			"--",
			"exec-plugin",
		)
	}

	goArgs := []string{"build", "-o", "plugin"}
	goArgs = append(goArgs, BuildFlags...)

	if err := runCmd("go", false, goArgs...); err != nil {
		return err
	}

	return runCmd("./plugin", true, "exec-plugin")
}

// NOTE: the below is from go-shellquote (MIT License)
// https://github.com/kballard/go-shellquote/blob/master/quote.go

// shellQuoteJoin quotes each argument and joins them with a space.
// If passed to /bin/sh, the resulting string will be split back into the
// original arguments.
func shellQuoteJoin(args ...string) string {
	var buf bytes.Buffer
	for i, arg := range args {
		if i != 0 {
			buf.WriteByte(' ')
		}
		quote(arg, &buf)
	}
	return buf.String()
}

const (
	specialChars      = "\\'\"`${[|&;<>()*?!"
	extraSpecialChars = " \t\n"
	prefixChars       = "~"
)

func quote(word string, buf *bytes.Buffer) {
	// We want to try to produce a "nice" output. As such, we will
	// backslash-escape most characters, but if we encounter a space, or if we
	// encounter an extra-special char (which doesn't work with
	// backslash-escaping) we switch over to quoting the whole word. We do this
	// with a space because it's typically easier for people to read multi-word
	// arguments when quoted with a space rather than with ugly backslashes
	// everywhere.
	origLen := buf.Len()

	if len(word) == 0 {
		// oops, no content
		buf.WriteString("''")
		return
	}

	cur, prev := word, word
	atStart := true
	for len(cur) > 0 {
		c, l := utf8.DecodeRuneInString(cur)
		cur = cur[l:]
		if strings.ContainsRune(specialChars, c) || (atStart && strings.ContainsRune(prefixChars, c)) {
			// copy the non-special chars up to this point
			if len(cur) < len(prev) {
				buf.WriteString(prev[0 : len(prev)-len(cur)-l])
			}
			buf.WriteByte('\\')
			buf.WriteRune(c)
			prev = cur
		} else if strings.ContainsRune(extraSpecialChars, c) {
			// start over in quote mode
			buf.Truncate(origLen)
			goto quote
		}
		atStart = false
	}
	if len(prev) > 0 {
		buf.WriteString(prev)
	}
	return

quote:
	// quote mode
	// Use single-quotes, but if we find a single-quote in the word, we need
	// to terminate the string, emit an escaped quote, and start the string up
	// again
	inQuote := false
	for len(word) > 0 {
		i := strings.IndexRune(word, '\'')
		if i == -1 {
			break
		}
		if i > 0 {
			if !inQuote {
				buf.WriteByte('\'')
				inQuote = true
			}
			buf.WriteString(word[0:i])
		}
		word = word[i+1:]
		if inQuote {
			buf.WriteByte('\'')
			inQuote = false
		}
		buf.WriteString("\\'")
	}
	if len(word) > 0 {
		if !inQuote {
			buf.WriteByte('\'')
		}
		buf.WriteString(word)
		buf.WriteByte('\'')
	}
}
