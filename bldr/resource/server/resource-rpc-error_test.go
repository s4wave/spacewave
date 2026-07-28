package resource_server_test

import (
	"context"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

type resourceRPCReceiptClient struct {
	srpc.Client
}

func (c *resourceRPCReceiptClient) ExecCall(
	ctx context.Context,
	serviceID, methodID string,
	in, out srpc.Message,
) error {
	if !resource.IsResourceRPCAdoptingUnaryMethod(serviceID, methodID) {
		return c.Client.ExecCall(ctx, serviceID, methodID, in, out)
	}

	receipt, err := srpc.ExecCallReceipt(ctx, c.Client, serviceID, methodID, in, out)
	if err != nil {
		return err
	}
	defer receipt.Close()
	return receipt.Commit()
}

type resourceRPCDeviceServer struct {
	reportErr     error
	checkoutErr   error
	checkoutCalls atomic.Int32
	releases      atomic.Int32
}

func (s *resourceRPCDeviceServer) WatchDeviceState(
	*s4wave_device.WatchDeviceStateRequest,
	s4wave_device.SRPCDeviceResourceService_WatchDeviceStateStream,
) error {
	return errors.New("watch unavailable")
}

func (s *resourceRPCDeviceServer) ReportDeviceStatus(
	ctx context.Context,
	_ *s4wave_device.ReportDeviceStatusRequest,
) (*s4wave_device.ReportDeviceStatusResponse, error) {
	owner, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := owner.AddResource(srpc.NewMux(), func() {
		s.releases.Add(1)
	}); err != nil {
		return nil, err
	}
	return nil, s.reportErr
}

func (s *resourceRPCDeviceServer) AccessCheckoutRoot(
	ctx context.Context,
	_ *s4wave_device.AccessCheckoutRootRequest,
) (*s4wave_device.AccessCheckoutRootResponse, error) {
	if s.checkoutCalls.Add(1) != 1 {
		return &s4wave_device.AccessCheckoutRootResponse{}, nil
	}
	owner, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := owner.AddResource(srpc.NewMux(), func() {
		s.releases.Add(1)
	}); err != nil {
		return nil, err
	}
	return nil, s.checkoutErr
}

func TestResourceRPCHandlerErrorFinalizesAdoption(t *testing.T) {
	reportErr := errors.New("device status reports are daemon-owned")
	checkoutErr := errors.New("checkout unavailable")
	deviceServer := &resourceRPCDeviceServer{
		reportErr:   reportErr,
		checkoutErr: checkoutErr,
	}
	rootMux := srpc.NewMux()
	if err := s4wave_device.SRPCRegisterDeviceResourceService(rootMux, deviceServer); err != nil {
		t.Fatalf("register device service: %v", err)
	}
	server := resource_server.NewResourceServer(rootMux)
	serverMux := srpc.NewMux()
	if err := server.Register(serverMux); err != nil {
		t.Fatalf("register resource server: %v", err)
	}
	service := resource.NewSRPCResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(serverMux))),
	)

	sessionCtx, cancelSession := context.WithCancel(t.Context())
	t.Cleanup(cancelSession)
	session, err := service.ResourceClient(sessionCtx, &resource.ResourceClientRequest{
		SupportsResourceAdoptionAck: true,
	})
	if err != nil {
		t.Fatalf("start resource client: %v", err)
	}
	event, err := session.Recv()
	if err != nil {
		t.Fatalf("receive resource client init: %v", err)
	}
	init := event.GetInit()
	if init == nil {
		t.Fatal("resource client did not send init")
	}
	if !init.GetSupportsResourceAdoptionAck() {
		t.Fatal("resource server did not enable requested adoption acknowledgments")
	}

	resourceClient := rpcstream.NewRpcStreamClient(
		func(ctx context.Context) (resource.SRPCResourceService_ResourceRpcClient, error) {
			return service.ResourceRpc(ctx)
		},
		strconv.FormatUint(uint64(init.GetRootResourceId()), 10),
		true,
	)
	deviceClient := s4wave_device.NewSRPCDeviceResourceServiceClient(
		&resourceRPCReceiptClient{Client: resourceClient},
	)

	callCtx, cancelCall := context.WithTimeout(t.Context(), time.Second)
	defer cancelCall()
	if _, err := deviceClient.ReportDeviceStatus(
		callCtx,
		&s4wave_device.ReportDeviceStatusRequest{},
	); err == nil || !strings.Contains(err.Error(), reportErr.Error()) {
		t.Fatalf("ReportDeviceStatus error: got %v, want %q", err, reportErr)
	}
	if got := deviceServer.releases.Load(); got != 1 {
		t.Fatalf("invocation resource releases: got %d, want 1", got)
	}
	if _, err := deviceClient.AccessCheckoutRoot(
		callCtx,
		&s4wave_device.AccessCheckoutRootRequest{},
	); err == nil || !strings.Contains(err.Error(), checkoutErr.Error()) {
		t.Fatalf("AccessCheckoutRoot error: got %v, want %q", err, checkoutErr)
	}
	if got := deviceServer.releases.Load(); got != 2 {
		t.Fatalf("invocation resource releases after adopting method: got %d, want 2", got)
	}
	if session.Context().Err() != nil {
		t.Fatalf("persistent resource client context: %v", session.Context().Err())
	}

	if _, err := deviceClient.AccessCheckoutRoot(
		t.Context(),
		&s4wave_device.AccessCheckoutRootRequest{},
	); err != nil {
		t.Fatalf("subsequent AccessCheckoutRoot: %v", err)
	}
}

// _ is a type assertion
var (
	_ srpc.Client                                   = (*resourceRPCReceiptClient)(nil)
	_ s4wave_device.SRPCDeviceResourceServiceServer = (*resourceRPCDeviceServer)(nil)
)
