package plugin_host_scheduler

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/s4wave/spacewave/net/crypto"
	bldr_peer "github.com/s4wave/spacewave/net/peer"
)

func TestBuildStartupWaitBudgetDefaultsAndParses(t *testing.T) {
	conf := &Config{}
	budget, err := conf.BuildStartupWaitBudget()
	if err != nil {
		t.Fatal(err)
	}
	if budget != DefaultStartupWaitBudget {
		t.Fatalf("budget = %v, want default %v", budget, DefaultStartupWaitBudget)
	}

	conf.StartupWaitBudgetDur = "30s"
	budget, err = conf.BuildStartupWaitBudget()
	if err != nil {
		t.Fatal(err)
	}
	if budget != 30*time.Second {
		t.Fatalf("budget = %v, want 30s", budget)
	}

	for _, invalid := range []string{"-1s", "bogus"} {
		conf.StartupWaitBudgetDur = invalid
		if _, err := conf.BuildStartupWaitBudget(); err == nil {
			t.Fatalf("startup_wait_budget_dur %q was accepted", invalid)
		}
	}
}

func TestConfigValidateRejectsNegativeStartupWaitBudget(t *testing.T) {
	_, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	peerID, err := bldr_peer.IDFromPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	conf := NewConfig("", "engine", "plugin-host", "volume", peerID.String(), false, false, false)
	if err := conf.Validate(); err != nil {
		t.Fatalf("baseline config failed validation: %v", err)
	}

	conf.StartupWaitBudgetDur = "-1s"
	if err := conf.Validate(); err == nil {
		t.Fatal("negative startup wait budget was accepted by Validate")
	}
}
