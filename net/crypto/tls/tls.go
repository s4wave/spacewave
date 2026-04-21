// Package p2ptls provides TLS identity for bifrost peer authentication.
//
// It generates x509 certificates with an embedded signed public key extension
// (OID 1.3.6.1.4.1.53594.1.1) that cryptographically ties the TLS certificate
// to a bifrost peer identity. Wire-compatible with go-libp2p's TLS handshake.
//
// Loosely based on the go-libp2p TLS implementation, covered under the MIT
// license: https://github.com/libp2p/go-libp2p/tree/master/p2p/security/tls
// Original reference commit: github.com/aperturerobotics/go-libp2p@5cfbb50b74e0
package p2ptls

import (
	gocrypto "crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"io"
	"math/big"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

const (
	certValidityPeriod = 100 * 365 * 24 * time.Hour // ~100 years
	certificatePrefix  = "libp2p-tls-handshake:"
)

var (
	extensionID       = getPrefixedExtensionID([]int{1, 1})
	extensionCritical bool // so we can mark the extension critical in tests
)

type signedKey struct {
	PubKey    []byte
	Signature []byte
}

// Identity is used to secure connections with TLS.
type Identity struct {
	config tls.Config
}

// IdentityConfig is used to configure an Identity.
type IdentityConfig struct {
	CertTemplate *x509.Certificate
	KeyLogWriter io.Writer
}

// IdentityOption transforms an IdentityConfig to apply optional settings.
type IdentityOption func(r *IdentityConfig)

// WithCertTemplate specifies the template to use when generating a new certificate.
func WithCertTemplate(template *x509.Certificate) IdentityOption {
	return func(c *IdentityConfig) {
		c.CertTemplate = template
	}
}

// WithKeyLogWriter optionally specifies a destination for TLS master secrets
// in NSS key log format that can be used to allow external programs
// such as Wireshark to decrypt TLS connections.
func WithKeyLogWriter(w io.Writer) IdentityOption {
	return func(c *IdentityConfig) {
		c.KeyLogWriter = w
	}
}

// NewIdentity creates a new TLS identity from a bifrost private key.
func NewIdentity(privKey crypto.PrivKey, opts ...IdentityOption) (*Identity, error) {
	config := IdentityConfig{}
	for _, opt := range opts {
		opt(&config)
	}

	var err error
	if config.CertTemplate == nil {
		config.CertTemplate, err = certTemplate()
		if err != nil {
			return nil, err
		}
	}

	cert, err := keyToCertificate(privKey, config.CertTemplate)
	if err != nil {
		return nil, err
	}
	return &Identity{
		config: tls.Config{
			MinVersion:         tls.VersionTLS13,
			InsecureSkipVerify: true, //nolint:gosec // Not insecure: we verify the cert chain ourselves.
			ClientAuth:         tls.RequireAnyClientCert,
			Certificates:       []tls.Certificate{*cert},
			VerifyConnection: func(_ tls.ConnectionState) error {
				panic("tls config not specialized for peer")
			},
			SessionTicketsDisabled: true,
			KeyLogWriter:           config.KeyLogWriter,
		},
	}, nil
}

// ConfigForPeer creates a new single-use tls.Config that verifies the peer's
// certificate chain and returns the peer's public key via the channel. If the
// peer ID is empty, the returned config will accept any peer.
func (i *Identity) ConfigForPeer(remote peer.ID) (*tls.Config, <-chan crypto.PubKey) {
	keyCh := make(chan crypto.PubKey, 1)
	conf := i.config.Clone()
	conf.VerifyConnection = func(cs tls.ConnectionState) (err error) {
		defer func() {
			if rerr := recover(); rerr != nil {
				err = errors.Errorf("panic processing peer certificate: %s", rerr)
			}
		}()
		defer close(keyCh)

		if len(cs.PeerCertificates) == 0 {
			return errors.New("expected peer certificate")
		}

		pubKey, err := PubKeyFromCertChain(cs.PeerCertificates)
		if err != nil {
			return err
		}
		if remote != "" && !remote.MatchesPublicKey(pubKey) {
			peerID, err := peer.IDFromPublicKey(pubKey)
			if err != nil {
				return errors.Errorf("peer ID mismatch: expected %s, could not determine actual: %s", remote, err)
			}
			return errors.Errorf("peer ID mismatch: expected %s, got %s", remote, peerID)
		}
		keyCh <- pubKey
		return nil
	}
	return conf, keyCh
}

// PubKeyFromCertChain verifies the certificate chain and extracts the remote's public key.
func PubKeyFromCertChain(chain []*x509.Certificate) (crypto.PubKey, error) {
	if len(chain) != 1 {
		return nil, errors.New("expected one certificate in the chain")
	}
	cert := chain[0]
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	var found bool
	var keyExt pkix.Extension
	for _, ext := range cert.Extensions {
		if extensionIDEqual(ext.Id, extensionID) {
			keyExt = ext
			found = true
			for idx, oident := range cert.UnhandledCriticalExtensions {
				if oident.Equal(ext.Id) {
					cert.UnhandledCriticalExtensions = append(cert.UnhandledCriticalExtensions[:idx], cert.UnhandledCriticalExtensions[idx+1:]...)
					break
				}
			}
			break
		}
	}
	if !found {
		return nil, errors.New("expected certificate to contain the key extension")
	}
	if _, err := cert.Verify(x509.VerifyOptions{Roots: pool}); err != nil {
		return nil, errors.Wrap(err, "certificate verification failed")
	}

	var sk signedKey
	if _, err := asn1.Unmarshal(keyExt.Value, &sk); err != nil {
		return nil, errors.Wrap(err, "unmarshalling signed certificate failed")
	}
	pubKey, err := crypto.UnmarshalPublicKey(sk.PubKey)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshalling public key failed")
	}
	certKeyPub, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return nil, err
	}
	valid, err := pubKey.Verify(append([]byte(certificatePrefix), certKeyPub...), sk.Signature)
	if err != nil {
		return nil, errors.Wrap(err, "signature verification failed")
	}
	if !valid {
		return nil, errors.New("signature invalid")
	}
	return pubKey, nil
}

// GenerateSignedExtension uses the provided private key to sign the public key,
// and returns the signature within a pkix.Extension. This extension is included
// in a certificate to cryptographically tie it to the bifrost private key.
func GenerateSignedExtension(sk crypto.PrivKey, pubKey gocrypto.PublicKey) (pkix.Extension, error) {
	keyBytes, err := crypto.MarshalPublicKey(sk.GetPublic())
	if err != nil {
		return pkix.Extension{}, err
	}
	certKeyPub, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return pkix.Extension{}, err
	}
	signature, err := sk.Sign(append([]byte(certificatePrefix), certKeyPub...))
	if err != nil {
		return pkix.Extension{}, err
	}
	value, err := asn1.Marshal(signedKey{
		PubKey:    keyBytes,
		Signature: signature,
	})
	if err != nil {
		return pkix.Extension{}, err
	}
	return pkix.Extension{Id: extensionID, Critical: extensionCritical, Value: value}, nil
}

// keyToCertificate generates a new ECDSA private key and corresponding x509 certificate.
// The certificate includes an extension that cryptographically ties it to the provided
// private key to authenticate TLS connections.
func keyToCertificate(sk crypto.PrivKey, certTmpl *x509.Certificate) (*tls.Certificate, error) {
	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	extension, err := GenerateSignedExtension(sk, certKey.Public())
	if err != nil {
		return nil, err
	}
	certTmpl.ExtraExtensions = append(certTmpl.ExtraExtensions, extension)

	certDER, err := x509.CreateCertificate(rand.Reader, certTmpl, certTmpl, certKey.Public(), certKey)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  certKey,
	}, nil
}

// certTemplate returns the template for generating an Identity's TLS certificates.
func certTemplate() (*x509.Certificate, error) {
	bigNum := big.NewInt(1 << 62)
	sn, err := rand.Int(rand.Reader, bigNum)
	if err != nil {
		return nil, err
	}

	subjectSN, err := rand.Int(rand.Reader, bigNum)
	if err != nil {
		return nil, err
	}

	return &x509.Certificate{
		SerialNumber: sn,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(certValidityPeriod),
		Subject:      pkix.Name{SerialNumber: subjectSN.String()},
	}, nil
}
