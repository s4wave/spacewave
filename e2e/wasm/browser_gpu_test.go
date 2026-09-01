//go:build !js

package wasm

import (
	"reflect"
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
)

func TestChromiumLaunchOptions(t *testing.T) {
	baseArgs := []string{
		"--allow-loopback-in-peer-connection",
		"--disable-features=WebRtcHideLocalIpsWithMdns",
	}
	for _, test := range []struct {
		name string
		env  string
		gpu  bool
	}{
		{name: "default", env: "", gpu: true},
		{name: "false", env: "false", gpu: false},
		{name: "true", env: "true", gpu: true},
		{name: "one", env: "1", gpu: true},
		{name: "yes", env: "yes", gpu: true},
		{name: "on", env: "on", gpu: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(chromiumGPUEnv, test.env)
			got := chromiumLaunchOptions(true)
			want := playwright.BrowserTypeLaunchOptions{
				Headless: new(true),
				Args:     baseArgs,
			}
			if test.gpu {
				channel := "chromium"
				want.Channel = &channel
				want.Headless = new(false)
				want.Args = append(want.Args,
					"--headless=new",
					"--ignore-gpu-blocklist",
					"--use-angle=vulkan",
					"--enable-gpu-rasterization",
					"--enable-zero-copy",
					"--enable-features=Vulkan",
				)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("launch options = %#v, want %#v", got, want)
			}
		})
	}
}
