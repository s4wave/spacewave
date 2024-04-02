package prof

import (
	"net/http"
	"net/http/pprof"
	"runtime"
	"time"

	"github.com/sirupsen/logrus"
)

// NewProfMux builds a new ServeMux with profiling endpoints.
func NewProfMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return mux
}

// ListenProf is a goroutine to listen on a profiling address.
func ListenProf(le *logrus.Entry, profListen string) error {
	le.Debugf("profiling listener running: %s", profListen)
	runtime.SetBlockProfileRate(1)
	runtime.SetMutexProfileFraction(1)
	mux := NewProfMux()
	server := &http.Server{Addr: profListen, Handler: mux, ReadTimeout: time.Second * 10}
	err := server.ListenAndServe()
	le.WithError(err).Warn("profiling listener exited")
	return err
}
