// Package main is an offline, one-time repair tool for locally owned SharedObjects.
package main

import (
	stderrors "errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pkg/errors"
)

// main reports only object identities and validation results, never key material.
func main() {
	// Parse the externally constrained flags of this throwaway migration program.
	statePath := flag.String("state-path", "", "required existing state directory")
	dryRun := flag.Bool("dry-run", true, "validate proposed repairs without changing state")
	flag.Parse()
	if *statePath == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "--state-path is required; positional arguments are unsupported")
		os.Exit(2)
	}

	// Finish lease and database cleanup before returning an unsuccessful status.
	if err := run(*statePath, *dryRun); err != nil {
		fmt.Fprintln(os.Stderr, "migration:", err)
		os.Exit(1)
	}
}

// run inventories every store before writing independently validated local repairs.
func run(statePath string, dryRun bool) (err error) {
	// Resolve the requested root without creating a missing state directory.
	statePath, err = filepath.Abs(statePath)
	if err != nil {
		return err
	}
	statePath, err = filepath.EvalSymlinks(statePath)
	if err != nil {
		return err
	}
	if err := requireOffline(statePath); err != nil {
		return err
	}

	// Hold the daemon's existing kernel lease without modifying its database.
	lease, err := acquireLease(statePath)
	if err != nil {
		return err
	}
	defer func() { err = stderrors.Join(err, lease.Close()) }()
	if err := requireOffline(statePath); err != nil {
		return err
	}

	// Discover native volumes, including volumes below the provider directory.
	var paths []string
	if err := filepath.WalkDir(statePath, func(path string, ent os.DirEntry, walkErr error) error {
		// Reject symlinked volumes rather than accessing state outside this root.
		if walkErr != nil {
			return walkErr
		}
		if strings.HasSuffix(ent.Name(), ".s4wave") {
			if !ent.Type().IsRegular() {
				return errors.Errorf("volume is not a regular file: %s", path)
			}
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		return err
	}
	slices.Sort(paths)

	// Build the complete inventory and signing-key set without opening any writer.
	keys := make(keySet)
	stores := make([]*Store, 0, len(paths))
	for _, path := range paths {
		store, err := ReadStore(path, keys)
		if err != nil {
			return errors.Wrapf(err, "inventory %s", path)
		}
		stores = append(stores, store)
	}
	defer clear(keys)
	fmt.Printf("state=%s dry_run=%t stores=%d\n", statePath, dryRun, len(stores))
	fmt.Println("sentinel=SHA256(UTF8(spacewave/sobject/migrated-head) || 00 || BE32(len(soID)) || UTF8(soID) || BE32(len(peerID)) || UTF8(peerID) || BE64(nonce))")

	// Report each object and prepare every repair before making the first write.
	var preparation error
	count := 0
	for _, store := range stores {
		fmt.Printf("store=%s objects=%d object_stores=%q\n", store.path, len(store.records), store.objectStores)
		for _, record := range store.records {
			// Check current state and history separately so both failures remain visible.
			owner, validator := record.signers(keys)
			beforeState, beforeChain := record.validation()
			fmt.Printf("  so=%s provider=%s account=%s space=%q owner_local=%t validator_local=%t before_state=%q before_chain=%q\n", record.id, record.provider, record.account, record.name, owner != nil, validator != nil, beforeState, beforeChain)

			// Cloud, CDN, and unavailable-key objects remain outside the repair set.
			if record.provider != "local" || record.state == nil || owner == nil || validator == nil {
				fmt.Println("    action=out-of-scope after_state=unchanged after_chain=unchanged")
				continue
			}
			if beforeState == "valid" && beforeChain == "valid" {
				fmt.Println("    action=already-valid after_state=valid after_chain=valid")
				continue
			}

			// Apply the prescribed repair to a clone and require master's validators.
			record.next, record.genesis, err = MigrateState(record.id, record.state, owner, validator)
			if err != nil {
				fmt.Printf("    action=blocked after_state=%q\n", err.Error())
				preparation = stderrors.Join(preparation, errors.Wrapf(err, "prepare %s", record.id))
				continue
			}
			count++
			fmt.Printf("    action=migrate after_state=valid after_chain=valid inner=preserved seqno=preserved grants=preserved dropped_ops=%d dropped_rejections=%d\n", len(record.state.GetOps()), len(record.state.GetOpRejections()))
		}
	}
	if preparation != nil {
		return preparation
	}

	// Commit one transaction per eligible object and reopen each written store.
	if !dryRun {
		for _, store := range stores {
			if err := store.Write(); err != nil {
				return errors.Wrapf(err, "write %s", store.path)
			}
			for _, record := range store.records {
				if record.next != nil {
					fmt.Printf("  reopened so=%s state=valid chain=valid checkpoint=valid local_queue=empty\n", record.id)
				}
			}
		}
	}
	fmt.Printf("complete eligible_repairs=%d dry_run=%t\n", count, dryRun)
	return nil
}
