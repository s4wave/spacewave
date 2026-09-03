//go:build !js

package harness

import (
	"context"
	"os"
	"strings"
	"sync"

	playwright "github.com/mxschmitt/playwright-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ChromiumGPUEnv selects the Chromium GPU launch policy for local e2e runs.
const ChromiumGPUEnv = "E2E_CHROMIUM_GPU"

// ChromiumGPUPreference is the three-state Chromium GPU launch policy parsed
// from E2E_CHROMIUM_GPU.
type ChromiumGPUPreference int

const (
	// ChromiumGPUPreferGPU is the unset default. Launch with hardware GPU
	// enabled; on a launch error, retry the legacy software launch once.
	ChromiumGPUPreferGPU ChromiumGPUPreference = iota
	// ChromiumGPULegacy is an explicit opt-out (false/0/no/off). Launch
	// legacy software Chromium only.
	ChromiumGPULegacy
	// ChromiumGPURequired is an explicit opt-in (true/1/yes/on). The GPU
	// launch is required and never falls back.
	ChromiumGPURequired
)

// ParseChromiumGPUPreference parses a raw E2E_CHROMIUM_GPU value.
func ParseChromiumGPUPreference(raw string) (ChromiumGPUPreference, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return ChromiumGPUPreferGPU, nil
	case "false", "0", "no", "off":
		return ChromiumGPULegacy, nil
	case "true", "1", "yes", "on":
		return ChromiumGPURequired, nil
	default:
		return 0, errors.Errorf("unsupported %s value %q", ChromiumGPUEnv, raw)
	}
}

// ResolveChromiumGPUPreference resolves the launch policy from the process
// environment. Harnesses call this once per boot.
func ResolveChromiumGPUPreference() (ChromiumGPUPreference, error) {
	return ParseChromiumGPUPreference(os.Getenv(ChromiumGPUEnv))
}

// String returns the stable log name of the preference.
func (p ChromiumGPUPreference) String() string {
	switch p {
	case ChromiumGPULegacy:
		return "legacy"
	case ChromiumGPURequired:
		return "gpu-required"
	default:
		return "prefer-gpu"
	}
}

// ChromiumLaunchOptions returns the Playwright launch options for one
// Chromium launch mode. gpu selects the full-Chromium hardware-GPU launch;
// otherwise the options are byte-identical to the legacy software launch.
func ChromiumLaunchOptions(headless bool, gpu bool) playwright.BrowserTypeLaunchOptions {
	opts := playwright.BrowserTypeLaunchOptions{
		Headless: new(headless),
		Args: []string{
			"--allow-loopback-in-peer-connection",
			"--disable-features=WebRtcHideLocalIpsWithMdns",
		},
	}
	if !gpu {
		return opts
	}
	channel := "chromium"
	opts.Channel = &channel
	opts.Headless = new(false)
	opts.Args = append(opts.Args,
		"--headless=new",
		"--ignore-gpu-blocklist",
		"--use-angle=vulkan",
		"--enable-gpu-rasterization",
		"--enable-zero-copy",
		"--enable-features=Vulkan",
	)
	return opts
}

// ChromiumLaunchPolicy carries the resolved E2E_CHROMIUM_GPU preference and
// the sticky launch choice for one harness boot. The zero value is invalid;
// use NewChromiumLaunchPolicy.
type ChromiumLaunchPolicy struct {
	le   *logrus.Entry
	pref ChromiumGPUPreference

	mu       sync.Mutex
	resolved bool
	gpu      bool
}

// NewChromiumLaunchPolicy resolves the policy from the environment once per
// harness boot and logs the chosen preference.
func NewChromiumLaunchPolicy(le *logrus.Entry) (*ChromiumLaunchPolicy, error) {
	pref, err := ResolveChromiumGPUPreference()
	if err != nil {
		return nil, err
	}
	le.WithField("preference", pref.String()).Info("resolved " + ChromiumGPUEnv + " launch policy")
	return &ChromiumLaunchPolicy{le: le, pref: pref}, nil
}

// LaunchChromium performs one Chromium launch under the sticky policy. launch
// receives true when the GPU options must be used. The first launch resolves
// the sticky GPU choice: prefer-GPU probes the GPU launch once and falls back
// to the legacy launch on error; legacy never attempts GPU; required never
// falls back. Concurrent callers serialize only while the choice is
// unresolved; relaunches reuse it without retrying. Cancellation aborts a
// launch that has not started yet.
func LaunchChromium[B any](
	ctx context.Context,
	p *ChromiumLaunchPolicy,
	launch func(gpu bool) (B, error),
) (B, error) {
	var zero B
	if p == nil {
		return zero, errors.New("nil ChromiumLaunchPolicy; resolve the policy at harness boot")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}

	// Fast path: the sticky choice is already known.
	if gpu, ok := p.stickyChoice(); ok {
		return launchChromium(ctx, gpu, launch)
	}

	// Serialize only the unresolved selection.
	p.mu.Lock()
	if p.resolved {
		gpu := p.gpu
		p.mu.Unlock()
		return launchChromium(ctx, gpu, launch)
	}
	if err := ctx.Err(); err != nil {
		p.mu.Unlock()
		return zero, err
	}

	switch p.pref {
	case ChromiumGPULegacy:
		p.rememberLocked(false)
		p.mu.Unlock()
		return launchChromium(ctx, false, launch)

	case ChromiumGPURequired:
		p.rememberLocked(true)
		p.mu.Unlock()
		b, err := launch(true)
		if err != nil {
			return zero, errors.Wrap(err, "launch chromium with GPU ("+ChromiumGPUEnv+"="+p.pref.String()+")")
		}
		return b, nil

	default:
		b, probeErr := launch(true)
		if probeErr == nil {
			p.rememberLocked(true)
			p.mu.Unlock()
			return b, nil
		}
		if ctx.Err() != nil {
			// The probe raced cancellation: stay unresolved so the next
			// boot retries the GPU launch instead of inheriting legacy.
			p.mu.Unlock()
			return zero, ctx.Err()
		}
		p.le.WithError(probeErr).Warn("chromium GPU launch failed; falling back to legacy software launch")
		p.rememberLocked(false)
		p.mu.Unlock()
		return launchChromium(ctx, false, launch)
	}
}

// stickyChoice returns the resolved GPU choice, if any.
func (p *ChromiumLaunchPolicy) stickyChoice() (bool, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.gpu, p.resolved
}

// rememberLocked records the sticky choice. Callers hold p.mu.
func (p *ChromiumLaunchPolicy) rememberLocked(gpu bool) {
	p.le.WithField("gpu", gpu).Info("chose chromium launch mode")
	p.resolved = true
	p.gpu = gpu
}

// launchChromium runs the injected launch function with the chosen mode.
func launchChromium[B any](ctx context.Context,
	gpu bool,
	launch func(gpu bool) (B, error),
) (B, error) {
	var zero B
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	b, err := launch(gpu)
	if err != nil {
		return zero, errors.Wrap(err, "launch chromium")
	}
	return b, nil
}
