//go:build !js && !wasip1

package s4wave_sql_world_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/s4db"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	gitea_testbed "github.com/s4wave/spacewave/internal/testbed/gitea"
	"github.com/s4wave/spacewave/testbed"
)

// scaleGitea grows the real application's issue table without retaining its rows.
// GITEA_SCALE opts in to expensive seed work. GITEA_PROFILE_DIR retains profiles.
func scaleGitea(t *testing.T, ctx context.Context, db *sql.DB, base, password string, issue int64, tb *testbed.Testbed) {
	// Read only the fixed schema, then build a bounded row-number source.
	t.Helper()
	rows, err := db.QueryContext(ctx, "SELECT * FROM gitea.issue LIMIT 0")
	if err != nil {
		t.Fatal(err)
	}
	columns, err := rows.Columns()
	_ = rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE gitea.scale_numbers (n INT PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	values := make([]string, 1000)
	for i := range values {
		values[i] = "(" + strconv.Itoa(i) + ")"
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO gitea.scale_numbers VALUES "+strings.Join(values, ",")); err != nil {
		t.Fatal(err)
	}

	// Clone a single issue's fields, assigning a distinct primary key and issue index.
	quoted, selected := make([]string, len(columns)), make([]string, len(columns))
	for i, name := range columns {
		quoted[i] = "`" + strings.ReplaceAll(name, "`", "``") + "`"
		selected[i] = "source." + quoted[i]
		if name == "id" || name == "index" {
			selected[i] = "? + numbers.n"
		}
	}
	seed := "INSERT INTO gitea.issue (" + strings.Join(quoted, ",") + ") SELECT " + strings.Join(selected, ",") + " FROM gitea.issue source CROSS JOIN gitea.scale_numbers numbers WHERE source.id = 1 AND numbers.n < ?"
	count := 1
	for _, size := range []int{100000, 1000000} {
		started := time.Now()
		for count < size {
			batch := min(1000, size-count)
			if _, err := db.ExecContext(ctx, seed, count+1, count+1, batch); err != nil {
				t.Fatal(err)
			}
			count += batch
		}
		if _, err := db.ExecContext(ctx, "UPDATE gitea.issue_index SET max_index = ? WHERE group_id = 1", size); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "UPDATE gitea.repository SET num_issues = ? WHERE id = 1", size); err != nil {
			t.Fatal(err)
		}
		t.Logf("Gitea seed rows=%d elapsed=%s", count, time.Since(started))
		measureGitea(t, ctx, db, base, password, issue, size, tb)
		if err := db.QueryRowContext(ctx, "SELECT MAX(id) FROM gitea.issue").Scan(&count); err != nil {
			t.Fatal(err)
		}
	}
}

// measureGitea profiles fixed-output HTTP reads and ten application mutations.
func measureGitea(t *testing.T, ctx context.Context, db *sql.DB, base, password string, issue int64, size int, tb *testbed.Testbed) {
	// Record the actual SQL plan rather than assuming the predicate is indexed.
	t.Helper()
	rows, err := db.QueryContext(ctx, "EXPLAIN FORMAT=tree SELECT content FROM gitea.issue WHERE repo_id = 1 AND `index` = "+strconv.FormatInt(issue, 10))
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		t.Logf("Gitea plan rows=%d %s", size, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()

	// Profile the adapter and storage process. Gitea is a separate process.
	if dir := os.Getenv("GITEA_PROFILE_DIR"); dir != "" {
		file, err := os.Create(filepath.Join(dir, fmt.Sprintf("sql-gitea-%d.cpu.pprof", size)))
		if err != nil {
			t.Fatal(err)
		}
		if err := pprof.StartCPUProfile(file); err != nil {
			t.Fatal(err)
		}
		writeGiteaAllocProfile(t, dir, size, "before")
		defer func() {
			pprof.StopCPUProfile()
			_ = file.Close()
			writeGiteaAllocProfile(t, dir, size, "after")
		}()
	}
	url := base + "/api/v1/repos/operator/proof/issues/" + strconv.FormatInt(issue, 10)
	gitea_testbed.Request(t, ctx, url, password, "GET", "")

	// Keep output and query count fixed while increasing stored issue count tenfold.
	for _, clients := range []int{1, 10} {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		readBefore := block.ContextReadCounter(ctx).Snapshot()
		started := time.Now()
		var workers sync.WaitGroup
		for range clients {
			workers.Go(func() {
				for range 100 / clients {
					gitea_testbed.Request(t, ctx, url, password, "GET", "")
				}
			})
		}
		workers.Wait()
		runtime.ReadMemStats(&after)
		readAfter := block.ContextReadCounter(ctx).Snapshot()
		t.Logf("Gitea read rows=%d clients=%d requests=100 elapsed=%s adapter_bytes=%d adapter_allocs=%d", size, clients, time.Since(started), after.TotalAlloc-before.TotalAlloc, after.Mallocs-before.Mallocs)
		t.Logf("Gitea read counters rows=%d clients=%d blocks=%d bytes=%d decoded=%d read_time=%s", size, clients, readAfter.BlockReadCount-readBefore.BlockReadCount, readAfter.BlockReadBytes-readBefore.BlockReadBytes, readAfter.DecodedBlockUnmarshalCount-readBefore.DecodedBlockUnmarshalCount, readAfter.BlockReadTime-readBefore.BlockReadTime)
	}

	// Record native publication work separately from the application latency.
	native := tb.Volume.(*volume_s4db.Volume).GetKvtxStore().(*s4db.DB)
	before, err := native.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seqBefore, err := tb.WorldState.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	readBefore := block.ContextReadCounter(ctx).Snapshot()

	// Time actual application issue creation, including World publication before OK.
	started := time.Now()
	for range 10 {
		gitea_testbed.Request(t, ctx, base+"/api/v1/repos/operator/proof/issues", password, "POST", `{"title":"Scale mutation","body":"bounded mutation"}`)
	}
	t.Logf("Gitea write rows=%d mutations=10 elapsed=%s", size, time.Since(started))
	readAfter := block.ContextReadCounter(ctx).Snapshot()
	seqAfter, err := tb.WorldState.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	after, err := native.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Gitea write counters rows=%d world_commits=%d blocks=%d bytes=%d native_written=%d native_user_bytes=%d flushes=%d background_wait_max_since_open=%s", size, seqAfter-seqBefore, readAfter.BlockReadCount-readBefore.BlockReadCount, readAfter.BlockReadBytes-readBefore.BlockReadBytes, after.Written-before.Written, after.UserBytes-before.UserBytes, after.Flushes-before.Flushes, after.BackgroundWaitMax)
}

// writeGiteaAllocProfile retains sampled allocation totals for profile subtraction.
func writeGiteaAllocProfile(t *testing.T, dir string, size int, phase string) {
	// Flush sampling before writing to a file outside the disposable database.
	t.Helper()
	runtime.GC()
	file, err := os.Create(filepath.Join(dir, fmt.Sprintf("sql-gitea-%d-%s.alloc.pprof", size, phase)))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := pprof.Lookup("allocs").WriteTo(file, 0); err != nil {
		t.Fatal(err)
	}
}
