package gocompiler

import (
	"os"
	"slices"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	// TinyGoProfileEnv selects the TinyGo build profile.
	TinyGoProfileEnv = "BLDR_TINYGO_PROFILE"
	// TinyGoOptEnv overrides the TinyGo optimization level.
	TinyGoOptEnv = "BLDR_TINYGO_OPT"
	// TinyGoPanicStrategyEnv controls the TinyGo panic strategy.
	TinyGoPanicStrategyEnv = "BLDR_TINYGO_PANIC"
	// TinyGoGCEnv overrides the TinyGo garbage collector.
	TinyGoGCEnv = "BLDR_TINYGO_GC"
	// TinyGoSchedulerEnv overrides the TinyGo scheduler.
	TinyGoSchedulerEnv = "BLDR_TINYGO_SCHEDULER"
	// TinyGoLLVMFeaturesEnv overrides the TinyGo LLVM feature list.
	TinyGoLLVMFeaturesEnv = "BLDR_TINYGO_LLVM_FEATURES"
	// TinyGoInterpTimeoutEnv overrides TinyGo interp optimization timeout.
	TinyGoInterpTimeoutEnv = "BLDR_TINYGO_INTERP_TIMEOUT"

	// TinyGoProfileDefault uses the optimized TinyGo defaults.
	TinyGoProfileDefault = "default"
	// TinyGoProfileFast uses a local development TinyGo profile.
	TinyGoProfileFast = "fast"
	// TinyGoProfileOptimized uses the optimized TinyGo profile.
	TinyGoProfileOptimized = "optimized"
)

var tinyGoStartupCacheEnvKeys = []string{
	TinyGoProfileEnv,
	TinyGoOptEnv,
	TinyGoPanicStrategyEnv,
	TinyGoGCEnv,
	TinyGoSchedulerEnv,
	TinyGoLLVMFeaturesEnv,
	TinyGoInterpTimeoutEnv,
}

// TinyGoOptions configures TinyGo compiler arguments.
type TinyGoOptions struct {
	// Profile selects the default argument set.
	Profile string
	// Opt overrides the optimization level.
	Opt string
	// PanicStrategy overrides the panic strategy.
	PanicStrategy string
	// GC overrides the garbage collector.
	GC string
	// Scheduler overrides the scheduler.
	Scheduler string
	// LLVMFeatures overrides the comma-separated LLVM feature list.
	LLVMFeatures string
	// InterpTimeout overrides the TinyGo interp optimization timeout.
	InterpTimeout string
}

// TinyGoStartupCacheEnvKeys returns env keys that affect TinyGo artifact identity.
func TinyGoStartupCacheEnvKeys() []string {
	return slices.Clone(tinyGoStartupCacheEnvKeys)
}

// TinyGoOptionsFromEnv resolves TinyGo options from the process environment.
func TinyGoOptionsFromEnv() TinyGoOptions {
	return TinyGoOptions{
		Profile:       os.Getenv(TinyGoProfileEnv),
		Opt:           os.Getenv(TinyGoOptEnv),
		PanicStrategy: os.Getenv(TinyGoPanicStrategyEnv),
		GC:            os.Getenv(TinyGoGCEnv),
		Scheduler:     os.Getenv(TinyGoSchedulerEnv),
		LLVMFeatures:  os.Getenv(TinyGoLLVMFeaturesEnv),
		InterpTimeout: os.Getenv(TinyGoInterpTimeoutEnv),
	}
}

// GetDefaultTinygoArgs returns the TinyGo args selected by environment.
func GetDefaultTinygoArgs() ([]string, error) {
	return TinyGoArgs(TinyGoOptionsFromEnv())
}

// TinyGoArgs converts TinyGoOptions into compiler args.
func TinyGoArgs(opts TinyGoOptions) ([]string, error) {
	profile, err := resolveTinyGoProfile(opts.Profile)
	if err != nil {
		return nil, err
	}

	opt, err := resolveTinyGoOpt(profile, opts.Opt)
	if err != nil {
		return nil, err
	}

	panicStrategy, err := resolveTinyGoPanicStrategy(opts.PanicStrategy)
	if err != nil {
		return nil, err
	}
	features, err := resolveTinyGoLLVMFeatures(opts.LLVMFeatures)
	if err != nil {
		return nil, err
	}

	args := []string{
		"-panic=" + panicStrategy,
		"-llvm-features=" + strings.Join(features, ","),
	}
	if opt != "" {
		args = append([]string{"-opt=" + opt}, args...)
	}
	gc, err := resolveTinyGoGC(profile, opts.GC)
	if err != nil {
		return nil, err
	}
	if gc != "" {
		args = append(args, "-gc="+gc)
	}
	if scheduler := strings.TrimSpace(opts.Scheduler); scheduler != "" {
		if err := validateTinyGoArgValue(TinyGoSchedulerEnv, scheduler); err != nil {
			return nil, err
		}
		args = append(args, "-scheduler="+scheduler)
	}
	interpTimeout, err := resolveTinyGoInterpTimeout(profile, opts.InterpTimeout)
	if err != nil {
		return nil, err
	}
	if interpTimeout != "" {
		args = append(args, "-interp-timeout="+interpTimeout)
	}
	return args, nil
}

// ResolveTinyGoPanicStrategy resolves the configured TinyGo panic strategy.
func ResolveTinyGoPanicStrategy() (string, error) {
	return resolveTinyGoPanicStrategy(os.Getenv(TinyGoPanicStrategyEnv))
}

func resolveTinyGoProfile(profile string) (string, error) {
	switch strings.TrimSpace(profile) {
	case "", TinyGoProfileDefault, TinyGoProfileOptimized:
		return TinyGoProfileOptimized, nil
	case TinyGoProfileFast:
		return TinyGoProfileFast, nil
	default:
		return "", errors.Errorf("unsupported %s value %q, expected default, fast, or optimized", TinyGoProfileEnv, profile)
	}
}

func resolveTinyGoOpt(profile, rawOpt string) (string, error) {
	opt := strings.TrimSpace(rawOpt)
	if opt == "" && profile == TinyGoProfileFast {
		opt = "1"
	}
	if opt == "" && profile == TinyGoProfileOptimized {
		opt = "2"
	}
	if opt == "" {
		return "", nil
	}
	if err := validateTinyGoOpt(opt); err != nil {
		return "", err
	}
	return opt, nil
}

func resolveTinyGoPanicStrategy(panicStrategy string) (string, error) {
	switch strings.TrimSpace(panicStrategy) {
	case "", "trap":
		return "trap", nil
	case "print":
		return "print", nil
	default:
		return "", errors.Errorf("unsupported %s value %q, expected trap or print", TinyGoPanicStrategyEnv, panicStrategy)
	}
}

func validateTinyGoOpt(opt string) error {
	switch opt {
	case "0", "1", "2", "s", "z":
		return nil
	default:
		return errors.Errorf("unsupported %s value %q, expected 0, 1, 2, s, or z", TinyGoOptEnv, opt)
	}
}

func resolveTinyGoGC(profile, rawGC string) (string, error) {
	gc := strings.TrimSpace(rawGC)
	if gc == "" && profile == TinyGoProfileFast {
		gc = "leaking"
	}
	if gc == "" {
		return "", nil
	}
	if err := validateTinyGoArgValue(TinyGoGCEnv, gc); err != nil {
		return "", err
	}
	return gc, nil
}

func resolveTinyGoLLVMFeatures(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return GetDefaultTinygoLlvmFeatures(), nil
	}
	features := strings.Split(raw, ",")
	for _, feature := range features {
		if feature == "" {
			return nil, errors.Errorf("unsupported %s value %q: empty feature", TinyGoLLVMFeaturesEnv, raw)
		}
		if err := validateTinyGoArgValue(TinyGoLLVMFeaturesEnv, feature); err != nil {
			return nil, err
		}
	}
	return features, nil
}

func validateTinyGoArgValue(envKey, value string) error {
	if strings.ContainsAny(value, " \t\r\n,") {
		return errors.Errorf("unsupported %s value %q: values cannot contain whitespace or commas", envKey, value)
	}
	return nil
}

func resolveTinyGoInterpTimeout(profile, rawTimeout string) (string, error) {
	timeout := strings.TrimSpace(rawTimeout)
	if timeout == "" && profile == TinyGoProfileFast {
		timeout = "10m"
	}
	if timeout == "" {
		return "", nil
	}
	if _, err := time.ParseDuration(timeout); err != nil {
		return "", errors.Wrapf(err, "unsupported %s value %q", TinyGoInterpTimeoutEnv, timeout)
	}
	return timeout, nil
}
