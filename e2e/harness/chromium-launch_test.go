//go:build !js

package harness

import (
	"context"
	"errors"
	"reflect"
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
	"github.com/sirupsen/logrus"
)

func TestParseChromiumGPUPreference(t *testing.T) {
	for _, test := range []struct {
		name string
		env  string
		want ChromiumGPUPreference
	}{
		{name: "unset", env: "", want: ChromiumGPUPreferGPU},
		{name: "false", env: "false", want: ChromiumGPULegacy},
		{name: "zero", env: "0", want: ChromiumGPULegacy},
		{name: "no", env: "no", want: ChromiumGPULegacy},
		{name: "off", env: "off", want: ChromiumGPULegacy},
		{name: "true", env: "true", want: ChromiumGPURequired},
		{name: "one", env: "1", want: ChromiumGPURequired},
		{name: "yes", env: "yes", want: ChromiumGPURequired},
		{name: "on", env: "on", want: ChromiumGPURequired},
		{name: "case and space", env: " True ", want: ChromiumGPURequired},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseChromiumGPUPreference(test.env)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("preference = %v, want %v", got, test.want)
			}
		})
	}

	for _, raw := range []string{"maybe", "2"} {
		if _, err := ParseChromiumGPUPreference(raw); err == nil {
			t.Errorf("ParseChromiumGPUPreference(%q) accepted an unsupported value", raw)
		}
	}
}

func TestResolveChromiumGPUPreferenceEnv(t *testing.T) {
	t.Setenv(ChromiumGPUEnv, "off")
	got, err := ResolveChromiumGPUPreference()
	if err != nil {
		t.Fatal(err)
	}
	if got != ChromiumGPULegacy {
		t.Fatalf("preference = %v, want %v", got, ChromiumGPULegacy)
	}
}

func TestChromiumLaunchOptions(t *testing.T) {
	baseArgs := []string{
		"--allow-loopback-in-peer-connection",
		"--disable-features=WebRtcHideLocalIpsWithMdns",
	}
	gpuArgs := append(append([]string{}, baseArgs...),
		"--headless=new",
		"--ignore-gpu-blocklist",
		"--use-angle=vulkan",
		"--enable-gpu-rasterization",
		"--enable-zero-copy",
		"--enable-features=Vulkan",
	)
	channel := "chromium"

	for _, test := range []struct {
		name     string
		headless bool
		gpu      bool
		want     playwright.BrowserTypeLaunchOptions
	}{
		{
			name:     "legacy headless",
			headless: true,
			want: playwright.BrowserTypeLaunchOptions{
				Headless: new(true),
				Args:     baseArgs,
			},
		},
		{
			name:     "legacy headed",
			headless: false,
			want: playwright.BrowserTypeLaunchOptions{
				Headless: new(false),
				Args:     baseArgs,
			},
		},
		{
			name:     "gpu",
			headless: true,
			gpu:      true,
			want: playwright.BrowserTypeLaunchOptions{
				Channel:  &channel,
				Headless: new(false),
				Args:     gpuArgs,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := ChromiumLaunchOptions(test.headless, test.gpu)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("launch options = %#v, want %#v", got, test.want)
			}
		})
	}
}

// launchRecorder records injected launch calls as gpu flags.
type launchRecorder[B any] struct {
	calls []bool
	fail  map[bool]error
	value B
}

func (r *launchRecorder[B]) launch(gpu bool) (B, error) {
	return r.launchRecord(gpu)
}

func newTestPolicy(pref ChromiumGPUPreference) *ChromiumLaunchPolicy {
	return &ChromiumLaunchPolicy{le: logrus.NewEntry(logrus.New()), pref: pref}
}

func TestLaunchChromiumPreferGPUFallsBackOnce(t *testing.T) {
	p := newTestPolicy(ChromiumGPUPreferGPU)
	rec := &launchRecorder[*int]{fail: map[bool]error{true: errors.New("boom")}}

	first, err := LaunchChromium(context.Background(), p, rec.launch)
	if err != nil {
		t.Fatal(err)
	}
	if first != rec.value {
		t.Fatalf("first launch returned %#v, want recorder value", first)
	}
	if !reflect.DeepEqual(rec.calls, []bool{true, false}) {
		t.Fatalf("first launch calls = %v, want [true false]", rec.calls)
	}

	// The fallback choice is sticky across relaunches.
	if _, err := LaunchChromium(context.Background(), p, rec.launch); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rec.calls, []bool{true, false, false}) {
		t.Fatalf("relaunch calls = %v, want no GPU retry", rec.calls)
	}
}

func TestLaunchChromiumPreferGPUSucceedsSticky(t *testing.T) {
	p := newTestPolicy(ChromiumGPUPreferGPU)
	rec := &launchRecorder[*int]{}

	if _, err := LaunchChromium(context.Background(), p, rec.launch); err != nil {
		t.Fatal(err)
	}
	if _, err := LaunchChromium(context.Background(), p, rec.launch); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rec.calls, []bool{true, true}) {
		t.Fatalf("calls = %v, want repeated GPU launches", rec.calls)
	}
}

func TestLaunchChromiumLegacyNeverAttemptsGPU(t *testing.T) {
	p := newTestPolicy(ChromiumGPULegacy)
	rec := &launchRecorder[*int]{fail: map[bool]error{false: errors.New("legacy boom")}}

	_, err := LaunchChromium(context.Background(), p, rec.launch)
	if err == nil {
		t.Fatal("expected the legacy launch error to surface")
	}
	if !errors.Is(err, rec.fail[false]) {
		t.Fatalf("error = %v, want wrapped legacy error", err)
	}
	if !reflect.DeepEqual(rec.calls, []bool{false}) {
		t.Fatalf("calls = %v, want a single legacy launch", rec.calls)
	}
}

func TestLaunchChromiumRequiredNoFallback(t *testing.T) {
	p := newTestPolicy(ChromiumGPURequired)
	rec := &launchRecorder[*int]{fail: map[bool]error{true: errors.New("gpu boom")}}

	_, err := LaunchChromium(context.Background(), p, rec.launch)
	if err == nil || !errors.Is(err, rec.fail[true]) {
		t.Fatalf("error = %v, want wrapped required-GPU error", err)
	}
	if !reflect.DeepEqual(rec.calls, []bool{true}) {
		t.Fatalf("calls = %v, want a single failed GPU launch with no retry", rec.calls)
	}
}

func TestLaunchChromiumCancelledProbeStaysUnresolved(t *testing.T) {
	p := newTestPolicy(ChromiumGPUPreferGPU)
	ctx, cancel := context.WithCancel(context.Background())
	rec := &launchRecorder[*int]{fail: map[bool]error{true: errors.New("probe boom")}}
	canceledLaunch := func(gpu bool) (*int, error) {
		// The probe returns after its context was canceled.
		cancel()
		return rec.launch(gpu)
	}

	if _, err := LaunchChromium(ctx, p, canceledLaunch); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if !reflect.DeepEqual(rec.calls, []bool{true}) {
		t.Fatalf("calls = %v, want a single GPU probe", rec.calls)
	}

	// The policy stays unresolved: a fresh context retries the GPU launch
	// instead of inheriting the legacy fallback.
	fresh := &launchRecorder[*int]{}
	if _, err := LaunchChromium(context.Background(), p, fresh.launch); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fresh.calls, []bool{true}) {
		t.Fatalf("fresh calls = %v, want the GPU probe retried", fresh.calls)
	}
}

func (r *launchRecorder[B]) launchRecord(gpu bool) (B, error) {
	r.calls = append(r.calls, gpu)
	if err := r.fail[gpu]; err != nil {
		var zero B
		return zero, err
	}
	return r.value, nil
}

func TestLaunchChromiumNilPolicyFailsLoudly(t *testing.T) {
	rec := &launchRecorder[*int]{}
	if _, err := LaunchChromium(context.Background(), nil, rec.launch); err == nil {
		t.Fatal("expected a nil policy to fail with an error")
	}
	if len(rec.calls) != 0 {
		t.Fatalf("calls = %v, want no launches", rec.calls)
	}
}

func TestLaunchChromiumCancelledBeforeLaunch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := newTestPolicy(ChromiumGPUPreferGPU)
	rec := &launchRecorder[*int]{}

	if _, err := LaunchChromium(ctx, p, rec.launch); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("calls = %v, want no launches", rec.calls)
	}
}
