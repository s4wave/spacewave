package provider_local

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/kvtx/hashmap"

	ftpserver "github.com/fclairamb/ftpserverlib"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/sirupsen/logrus"
	"github.com/spf13/afero"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
)

// ftpsTestServer is an in-process FTP server on a memory filesystem that
// requires explicit TLS and the login user and pass.
type ftpsTestServer struct {
	fs       afero.Fs
	tlsConf  *tls.Config
	listener net.Listener
}

// startFTPSTestServer serves a memory filesystem until the test ends. It
// returns the server, its port and a client TLS configuration that trusts it.
func startFTPSTestServer(t *testing.T) (*ftpsTestServer, uint32, *tls.Config) {
	t.Helper()

	// Issue a self-signed certificate for 127.0.0.1.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)

	// Serve until the test ends.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &ftpsTestServer{
		fs:       afero.NewMemMapFs(),
		tlsConf:  &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}},
		listener: listener,
	}
	ftp := ftpserver.NewFtpServer(srv)
	go func() { _ = ftp.ListenAndServe() }()
	t.Cleanup(func() { _ = ftp.Stop() })
	return srv, uint32(listener.Addr().(*net.TCPAddr).Port), &tls.Config{RootCAs: roots} //nolint:gosec // a port fits.
}

func (s *ftpsTestServer) GetSettings() (*ftpserver.Settings, error) {
	return &ftpserver.Settings{Listener: s.listener, TLSRequired: ftpserver.MandatoryEncryption}, nil
}

func (s *ftpsTestServer) ClientConnected(ftpserver.ClientContext) (string, error) { return "test", nil }
func (s *ftpsTestServer) ClientDisconnected(ftpserver.ClientContext)              {}
func (s *ftpsTestServer) GetTLSConfig() (*tls.Config, error)                      { return s.tlsConf, nil }

func (s *ftpsTestServer) AuthUser(_ ftpserver.ClientContext, user, pass string) (ftpserver.ClientDriver, error) {
	if user != "user" || pass != "pass" {
		return nil, errors.New("bad login")
	}
	return s.fs, nil
}

// TestFTPSPlacementAndFallback places a store on an FTPS backend with an SQL
// fallback. A block written and uploaded lands in a pack under the location's
// root and prefix and reads back by range after eviction through a pack store
// opened fresh, and a block only the fallback holds reads from it, fills the
// disk with a marker and uploads to the FTPS packs.
func TestFTPSPlacementAndFallback(t *testing.T) {
	// Start the server and write the netrc login of its host.
	srv, port, tlsConf := startFTPSTestServer(t)
	netrc := filepath.Join(t.TempDir(), "netrc")
	if err := os.WriteFile(netrc, []byte("machine 127.0.0.1 login user password pass\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	location := &account_settings.FtpsLocation{
		Host:      "127.0.0.1",
		Port:      port,
		NetrcPath: netrc,
		Root:      "hyperline-packs",
		Prefix:    "world/packs/",
		MaxConns:  2,
	}
	backend := &account_settings.StorageBackend{Id: "ftps", DisplayName: "packs", Ftps: location}
	if err := location.Validate(); err != nil {
		t.Fatal(err)
	}
	f := newFallbackTestOn(t, func() backendStore {
		store, err := buildFTPSBlockStore(logrus.NewEntry(logrus.New()), backend.GetFtps(), tlsConf, hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]()))
		if err != nil {
			t.Fatal(err)
		}
		return store
	})

	// Upload and evict a block, then read it back by range.
	data := []byte("block in an ftps pack")
	ref, _, err := f.wb.PutBlock(f.ctx, data, &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	f.upload()
	if result := f.evict(); result.Evicted != 1 {
		t.Fatalf("evict = %+v, want the block evicted", result)
	}
	if !f.packHolds(ref) {
		t.Fatal("a fresh pack store lacks the uploaded block")
	}

	// An SFTP location whose server refuses connections lists the same packs
	// over its FTPS port.
	refused, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = refused.Close()
	viaSFTP := location.CloneVT()
	viaSFTP.Sftp, viaSFTP.Port, viaSFTP.FtpsPort = true, uint32(refused.Addr().(*net.TCPAddr).Port), port //nolint:gosec // a TCP port fits.
	client, err := buildFTPSClient(logrus.NewEntry(logrus.New()), viaSFTP, tlsConf)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var listed int
	if err := client.ListObjects(f.ctx, location.GetRoot(), location.GetPrefix(), func(string, int64) error {
		listed++
		return nil
	}); err != nil || listed == 0 {
		t.Fatalf("list over the FTPS port = %d packs, %v; want the uploaded pack", listed, err)
	}

	// Count a tracked read after eviction separately from a first cache fill.
	f.tracked.track(ref)
	f.read(ref, data)
	f.waitResident(int64(len(data)))

	// A migrated parent has no local graph entry. Its cache fill must retain
	// child refs and enter the same resident budget as the evicted block.
	parentData := []byte("parent only in an ftps pack")
	parent, _, err := f.packs.PutBlock(f.ctx, parentData, &block.PutOpts{Refs: []*block.BlockRef{ref}, RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	f.read(parent, parentData)
	wantResident := int64(len(data) + len(parentData))
	f.waitResident(wantResident)

	// A first-time fill preserves child refs without inflating Refills.
	if status := f.status(); status.Refills != 1 {
		t.Fatalf("refills = %d, want only the tracked block read after eviction", status.Refills)
	}
	stored, err := f.local.GetStoredBlock(f.ctx, parent)
	if err != nil || stored == nil || !stored.RefsKnown || len(stored.Refs) != 1 || !stored.Refs[0].GetHash().CompareHash(ref.GetHash()) {
		t.Fatalf("cached parent = %+v, %v; want its child ref", stored, err)
	}

	// A warm read needs neither the FTPS backend nor the SQL fallback.
	func() {
		packs := f.packs
		f.packs = nil
		defer func() { f.packs = packs }()
		f.read(parent, parentData)
	}()
	if n := f.sql.reads.Load(); n != 0 {
		t.Fatalf("warm read touched the SQL fallback %d times", n)
	}

	// Reopen writeback over the same volume: both cache values must remain
	// accounted for and evictable, without having queued another upload.
	vol := f.local.(ownedStore).Volume
	markers, release, err := vol.AccessObjectStore(f.ctx, "test/markers", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Reload the counters from the saved eviction queue.
	reopened, err := block_store_writeback.NewStore(f.ctx, f.local, vol, markers, "test/markers", true)
	if err != nil {
		t.Fatal(err)
	}
	f.wb, f.p.wb = reopened, reopened
	if status := f.status(); status.Pending != 0 || status.ResidentBytes() != wantResident {
		t.Fatalf("reopened status = %+v, want %d resident bytes and no uploads", status, wantResident)
	}
	if result := f.evict(); result.Evicted != 2 || f.held(ref) || f.held(parent) {
		t.Fatalf("second eviction = %+v, want both cached values dropped", result)
	}

	// The next read fetches the parent again and accounts for its new copy.
	f.read(parent, parentData)
	f.waitResident(int64(len(parentData)))

	// The pack sits where the pack tool writes its packs.
	var packs []string
	_ = afero.Walk(srv.fs, "/", func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			packs = append(packs, path)
		}
		return nil
	})
	if len(packs) == 0 || !strings.HasPrefix(strings.TrimPrefix(packs[0], "/"), "hyperline-packs/world/packs/") {
		t.Fatalf("packs = %v, want one under hyperline-packs/world/packs/", packs)
	}

	// A block only the fallback holds reads, fills with a marker, and uploads.
	sqlData := []byte("block only in sql")
	sqlRef := f.putSQL(sqlData)
	f.read(sqlRef, sqlData)
	f.waitHeld(sqlRef)
	if status := f.status(); status.Pending != 1 {
		t.Fatalf("pending = %d, want the fallback block", status.Pending)
	}
	if f.packHolds(sqlRef) {
		t.Fatal("ftps packs hold the fallback block before its upload")
	}
	f.upload()
	if !f.packHolds(sqlRef) {
		t.Fatal("ftps packs lack the fallback block after the upload")
	}
}
