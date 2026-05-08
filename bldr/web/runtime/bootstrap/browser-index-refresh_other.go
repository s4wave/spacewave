//go:build !js

package web_runtime_bootstrap

import "github.com/sirupsen/logrus"

func triggerBrowserIndexCacheRefresh(_ *logrus.Entry) {}
