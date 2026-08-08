package web_runtime

import (
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

type webWorkerHostInvoker struct {
	webWorkerID string
	invoker     srpc.Invoker
}

func (w *webWorkerHostInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	ok, err := w.invoker.InvokeMethod(serviceID, methodID, strm)
	if err != nil {
		return ok, errors.Wrapf(err, "web-worker/%s %s/%s", w.webWorkerID, serviceID, methodID)
	}
	return ok, nil
}

// _ is a type assertion.
var _ srpc.Invoker = (*webWorkerHostInvoker)(nil)
