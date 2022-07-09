package web_runtime_controller

import (
	"encoding/base64"
	"net/http"

	"github.com/sirupsen/logrus"
)

func getTestPng() []byte {
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAACAAAAAgEAIAAACsiDHgAAAABGdBTUEAAYagMeiWXwAAAOVJREFUeJzVlsEKgzAQRKfgQX/Lfrf9rfaWHgYDkoYmZpPMehiGReQ91qCPEEIAPi/gmu9kcnN+GD0nM1/O4vNad7cC6850KHCiM5fz7fJwXdEBYPOygV/o7PICeXSmsMA/dKbkGShD51xsAzXo7DIC9ehMAYG76MypZ6ANnfNJG7BAZx8uYIfOHChgjR4F+MfuDx0AtmfnDfREZ+8m0B+9m8Ao9Chg9x0Yi877jTYwA529WWAeerPAbPQoUH8GNNA5r9yAEjp7sYAeerGAKnoUyJ8BbXTOMxvwgM6eCPhBTwS8oTO/5kL+Xge7xOwAAAAASUVORK5CYII=")
	return data
}

func getTestSwMux(le *logrus.Entry) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/b/test.png", http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		le.Debugf("service worker fetch test png: %s", req.URL.String())
		// TODO: Demo image
		rw.Header().Set("Content-Type", "image/png")
		rw.WriteHeader(200)
		// basic test image
		rw.Write(getTestPng())
	}))

	// TODO DEMO
	mux.Handle("/b/test.js", http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		le.Debugf("service worker fetch test component: %s", req.URL.String())
		// TODO: Demo image
		rw.Header().Set("Content-Type", "text/javascript")
		rw.WriteHeader(200)
		rw.Write([]byte(getTestComponentJS() + "\n"))
	}))
	return mux
}
