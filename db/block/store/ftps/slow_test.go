package block_store_ftps

import (
	"fmt"
	"testing"

	"github.com/jlaffaye/ftp"
)

// TestSlowTextConn reads the library's control reader of a live session
// through textConn and sends NOOP on it: the pinned jlaffaye/ftp still has
// the field slowClose needs, so a library bump that drops it fails here, not
// in a run, and the session stays in step for the library's own NOOP.
func TestSlowTextConn(t *testing.T) {
	// Run a call on a live session tracked under CloseOnDone.
	addr, tlsConf := startTestServer(t)
	c := NewClient(Config{Addr: addr, Logins: []Login{{User: "user", Password: "pass"}}, TLSConfig: tlsConf, MaxSessions: 1, CloseOnDone: true})
	t.Cleanup(c.Close)
	err := c.do(t.Context(), func(conn *ftp.ServerConn, _ *slot) error {
		// Find the reader and the tracked session.
		tc := textConn(conn)
		if tc == nil || c.sess.lookup(conn) == nil {
			return fmt.Errorf("textConn %v, session tracked %v", tc != nil, c.sess.lookup(conn) != nil)
		}

		// Send NOOP on it, then one through the library.
		if _, err := tc.Cmd("NOOP"); err != nil {
			return err
		}
		if _, _, err := tc.ReadResponse(ftp.StatusCommandOK); err != nil {
			return err
		}
		return conn.NoOp()
	})
	if err != nil {
		t.Fatal(err)
	}
}
