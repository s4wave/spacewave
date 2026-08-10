package auth_method_password

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	auth_method "github.com/s4wave/spacewave/auth/method"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/scrypt"
)

// MethodID is the auth method ID.
const MethodID = "password"

// ControllerID is the auth method controller ID.
const ControllerID = "auth/method/" + MethodID

// MaxDerivationMemoryBytes is the process memory budget for password derivation.
// A default scrypt derivation consumes the complete budget, so derivations run
// one at a time through the lifecycle-owned PasswordMethod.
const MaxDerivationMemoryBytes uint64 = 128 * DefaultScryptR * (1 << DefaultScryptN)

// maxEncodedParametersLen bounds persisted input before protobuf decoding can copy fields.
// Repository-produced records are at most 26 bytes.
const maxEncodedParametersLen = 64

// Version is the version of the password method implementation.
var Version = controller.MustParseVersion("0.1.0")

type scryptFunc func([]byte, []byte, int, int, int, int) ([]byte, error)

// PasswordMethod derives password keys within a fixed memory and lifecycle boundary.
type PasswordMethod struct {
	ctx       context.Context
	cancel    context.CancelFunc
	admission chan struct{}
	derive    scryptFunc

	mtx    sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

// NewPasswordMethod constructs a password method bound to ctx.
func NewPasswordMethod(ctx context.Context) *PasswordMethod {
	ownerCtx, cancel := context.WithCancel(ctx)
	return &PasswordMethod{
		ctx:       ownerCtx,
		cancel:    cancel,
		admission: make(chan struct{}, 1),
		derive:    scrypt.Key,
	}
}

// NewMethod constructs the password method as an auth method.
func NewMethod(ctx context.Context, le *logrus.Entry, handler auth_method.Handler) (auth_method.Method, error) {
	return NewPasswordMethod(ctx), nil
}

// GetMethodID returns the auth method ID.
func (p *PasswordMethod) GetMethodID() string { return MethodID }

// UnmarshalParameters unmarshals and validates untrusted persisted parameters.
func (p *PasswordMethod) UnmarshalParameters(data []byte) (auth_method.Parameters, error) {
	if len(data) > maxEncodedParametersLen {
		return nil, errors.Errorf("password parameters exceed %d bytes", maxEncodedParametersLen)
	}
	params := &Parameters{}
	if err := params.UnmarshalVT(data); err != nil {
		return nil, err
	}
	if err := params.Validate(); err != nil {
		return nil, err
	}
	return params, nil
}

// Authenticate authenticates with existing persisted parameters.
func (p *PasswordMethod) Authenticate(ctx context.Context, paramsi auth_method.Parameters, authSecretData []byte) (crypto.PrivKey, error) {
	params, ok := paramsi.(*Parameters)
	if !ok {
		return nil, errors.New("params object not recognized")
	}
	if len(authSecretData) == 0 {
		return nil, errors.New("auth secret data must be set")
	}
	return p.deriveKey(ctx, params, authSecretData)
}

// BuildParametersWithUsernamePassword creates trusted default parameters and derives their key.
func (p *PasswordMethod) BuildParametersWithUsernamePassword(ctx context.Context, username string, password []byte) (*Parameters, crypto.PrivKey, error) {
	return p.buildParametersWithUsernamePassword(ctx, username, password, DefaultScryptN, DefaultScryptR, DefaultScryptP)
}

func (p *PasswordMethod) buildParametersWithUsernamePassword(ctx context.Context, username string, password []byte, n, r, parallel uint32) (*Parameters, crypto.PrivKey, error) {
	params := newParameters(username, n, r, parallel)
	privKey, err := p.deriveKey(ctx, params, password)
	if err != nil {
		return nil, nil, err
	}
	return params, privKey, nil
}

func (p *PasswordMethod) admit(ctx context.Context) (func(), error) {
	callCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(p.ctx, cancel)
	defer func() {
		if stop() {
			cancel()
		}
	}()

	select {
	case p.admission <- struct{}{}:
	case <-callCtx.Done():
		return nil, callCtx.Err()
	}

	p.mtx.Lock()
	if p.closed {
		p.mtx.Unlock()
		<-p.admission
		return nil, context.Canceled
	}
	p.wg.Add(1)
	p.mtx.Unlock()

	return func() {
		<-p.admission
		p.wg.Done()
	}, nil
}

// Execute executes the auth method.
func (p *PasswordMethod) Execute(ctx context.Context) error { return nil }

// Close cancels waiting derivations and joins work already admitted.
func (p *PasswordMethod) Close() {
	p.mtx.Lock()
	if !p.closed {
		p.closed = true
		p.cancel()
	}
	p.mtx.Unlock()
	p.wg.Wait()
}

var (
	_ auth_method.Constructor = NewMethod
	_ auth_method.Method      = (*PasswordMethod)(nil)
)
