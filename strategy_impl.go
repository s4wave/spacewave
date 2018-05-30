package auth

import (
	"sync"

	"github.com/pkg/errors"
)

// ErrDuplicateImpl is returned when a duplicate auth implementation is registered.
var ErrDuplicateImpl = errors.New("duplicate auth strategy implementation")

// Strategy is an implementation of an auth strategy.
type Strategy interface {
	// GetAuthType returns the auth type this implementation satisfies.
	GetAuthType() AuthType
	// ValidateCredentials validates a set of credentials.
}

// authImplsMtx is the mutex on the authImpls map
var authImplsMtx sync.RWMutex

// authImpls contains registered implementations.
var authImpls = make(map[AuthType]Strategy)

// MustRegisterStrategy registers an encryption implementation or panics.
// expected to be called from Init(), but can be deferred
func MustRegisterStrategy(impl Strategy) {
	if err := RegisterStrategy(impl); err != nil {
		panic(err)
	}
}

// RegisterStrategy registers an encryption implementation.
// expected to be called from Init(), but can be deferred
func RegisterStrategy(impl Strategy) error {
	authType := impl.GetAuthType()

	authImplsMtx.Lock()
	defer authImplsMtx.Unlock()

	if _, ok := authImpls[authType]; ok {
		return ErrDuplicateImpl
	}

	authImpls[authType] = impl
	return nil
}

// GetStrategy returns the registered implementation of the type.
func GetStrategy(kind AuthType) (impl Strategy, err error) {
	if _, ok := AuthType_name[int32(kind)]; !ok {
		return nil, errors.Errorf("auth type unknown: %v", kind.String())
	}

	authImplsMtx.RLock()
	impl = authImpls[kind]
	authImplsMtx.RUnlock()

	if impl == nil {
		err = errors.Errorf("unimplemented auth type: %v", kind.String())
	}

	return
}
