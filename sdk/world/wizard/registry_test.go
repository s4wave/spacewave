package s4wave_wizard

import "testing"

func TestForgeClusterWizardDefaultNameIsDNS1123(t *testing.T) {
	wizard := LookupObjectWizard("forge/cluster")
	if wizard == nil {
		t.Fatal("forge/cluster wizard is not registered")
	}
	if got, want := wizard.GetDefaultNamePattern(), "cluster"; got != want {
		t.Fatalf("default name = %q, want %q", got, want)
	}
}
