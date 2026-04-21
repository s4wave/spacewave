package daemon_prof

import (
	"net/http"
	"net/http/pprof"
	"runtime"
	"time"

	"github.com/sirupsen/logrus"
)

func ListenProf(le *logrus.Entry, profListen string) error {
	runtime.SetBlockProfileRate(1)
	runtime.SetMutexProfileFraction(1)
	le.Debugf("profiling listener running: %s", profListen)
	mux := http.NewServeMux()

	// Register pprof handlers
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// Manually add support for paths linked to by index page at /debug/pprof/
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))

	server := &http.Server{Addr: profListen, Handler: mux, ReadHeaderTimeout: time.Second * 10}
	err := server.ListenAndServe()
	le.WithError(err).Warn("profiling listener exited")
	return err
}
