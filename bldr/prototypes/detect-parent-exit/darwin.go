//go:build darwin

package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	parentPid := os.Getppid()
	fmt.Printf("Monitoring parent process with PID: %d\n", parentPid)

	// Create kqueue
	kq, err := syscall.Kqueue()
	if err != nil {
		log.Fatalf("failed to create kqueue: %v", err)
	}

	// Set up the kevent to monitor for the exit of the parent process
	event := syscall.Kevent_t{
		Ident:  uint64(parentPid), //nolint:gosec
		Filter: syscall.EVFILT_PROC,
		Flags:  syscall.EV_ADD | syscall.EV_ENABLE,
		Fflags: syscall.NOTE_EXIT,
	}

	// Register the event
	events := []syscall.Kevent_t{event}
	nev, err := syscall.Kevent(kq, events, nil, nil)
	if err != nil {
		log.Fatalf("failed to register event: %v", err)
	}
	if nev != 0 {
		log.Fatalf("unexpected number of events: %d", nev)
	}

	// Create a signal handler for clean exit on interrupt
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		fmt.Printf("Received signal: %s\n", sig)
		syscall.Close(kq)
		os.Exit(0)
	}()

	// Monitor the kqueue
	for {
		events := make([]syscall.Kevent_t, 1)
		nev, err := syscall.Kevent(kq, nil, events, nil)
		if err != nil {
			log.Fatalf("failed to wait on kqueue: %v", err)
		}
		if nev > 0 {
			for _, e := range events {
				fmt.Printf("event: %#v\n", e)
				if e.Filter == syscall.EVFILT_PROC && e.Fflags&syscall.NOTE_EXIT != 0 {
					fmt.Printf("Parent process (pid %d) exited. Exiting...\n", parentPid)
					syscall.Close(kq)
					os.Exit(0)
				}
			}
		}
	}
}
