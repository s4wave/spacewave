//go:build !js

package harness

import (
	"context"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type resolveHooks struct {
	afterSnapshot func()
}

// Resolve returns a valid artifact, coalescing builds across processes.
func Resolve[A any](ctx context.Context, le *logrus.Entry, opts ResolveOptions, shape Shape[A]) (A, error) {
	return resolve(ctx, le, opts, shape, resolveHooks{})
}

func resolve[A any](ctx context.Context, le *logrus.Entry, opts ResolveOptions, shape Shape[A], hooks resolveHooks) (A, error) {
	var zero A

	// Compute the artifact content key and inspect existing generations.
	key, err := shape.ContentKey(ctx)
	if err != nil {
		return zero, err
	}
	preLock, preSet, err := lookupGenerations(ctx, shape, key)
	if err != nil {
		return zero, err
	}
	if !opts.RequireFresh && len(preLock) != 0 {
		return preLock[0].Artifact, nil
	}
	if hooks.afterSnapshot != nil {
		hooks.afterSnapshot()
	}

	// Acquire the build lock before rechecking concurrent generations.
	lock, err := AcquireBuildLock(ctx, le, opts.LockDir, opts.LockName)
	if err != nil {
		return zero, err
	}
	defer lock.Release()

	// Recheck generations created while waiting for the lock.
	post, _, err := lookupGenerations(ctx, shape, key)
	if err != nil {
		return zero, err
	}
	for _, generation := range post {
		if !opts.RequireFresh {
			return generation.Artifact, nil
		}
		if _, exists := preSet[generation.Token]; !exists {
			return generation.Artifact, nil
		}
	}

	// Build and validate a new artifact generation.
	generation, err := shape.Build(ctx, key)
	if err != nil {
		return zero, err
	}
	if generation.Token == "" {
		return zero, errors.New("artifact build returned an empty generation token")
	}
	return generation.Artifact, nil
}

func lookupGenerations[A any](ctx context.Context, shape Shape[A], key string) ([]Generation[A], map[string]struct{}, error) {
	// Load generations for the requested artifact key.
	generations, err := shape.Lookup(ctx, key)
	if err != nil {
		return nil, nil, err
	}

	// Validate generation tokens and index them for freshness checks.
	tokens := make(map[string]struct{}, len(generations))
	for _, generation := range generations {
		if generation.Token == "" {
			return nil, nil, errors.New("artifact lookup returned an empty generation token")
		}
		if _, exists := tokens[generation.Token]; exists {
			return nil, nil, errors.Errorf("artifact lookup returned duplicate generation token %q", generation.Token)
		}
		tokens[generation.Token] = struct{}{}
	}
	return generations, tokens, nil
}
