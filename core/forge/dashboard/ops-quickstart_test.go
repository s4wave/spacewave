package forge_dashboard

import (
	"crypto/rand"
	"testing"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_task "github.com/s4wave/spacewave/forge/task"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/testbed"
)

func generateQuickstartTestPeerID(t *testing.T) peer.ID {
	t.Helper()

	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

func TestInitForgeQuickstartSeedsExecutableTargets(t *testing.T) {
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}

	pid := generateQuickstartTestPeerID(t)
	op := &InitForgeQuickstartOp{
		LayoutKey:     "forge",
		DashboardKey:  "dashboard",
		ClusterKey:    "cluster",
		ClusterName:   "default",
		WorkerKey:     "session-worker",
		SessionPeerId: pid.String(),
		Timestamp:     timestamppb.Now(),
	}
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, op, pid); err != nil {
		t.Fatalf("ApplyWorldOp: %v", err)
	}

	taskKeys, err := forge_job.ListJobTasks(ctx, tb.WorldState, "sample-job")
	if err != nil {
		t.Fatalf("ListJobTasks: %v", err)
	}
	if len(taskKeys) != 3 {
		t.Fatalf("expected 3 quickstart tasks, got %d", len(taskKeys))
	}

	for _, taskKey := range taskKeys {
		target, _, err := forge_task.LookupTaskTarget(ctx, tb.WorldState, taskKey)
		if err != nil {
			t.Fatalf("LookupTaskTarget(%s): %v", taskKey, err)
		}
		if target.GetExec().GetDisable() {
			t.Fatalf("expected executable target for %s", taskKey)
		}
		if got := target.GetExec().GetController().GetId(); got != space_exec.NoopConfigID {
			t.Fatalf("expected %s controller for %s, got %q", space_exec.NoopConfigID, taskKey, got)
		}
	}
}
