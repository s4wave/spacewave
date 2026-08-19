package plugin_host

import "testing"

func TestPluginHostServerResolvesCallerInstance(t *testing.T) {
	s := &PluginHostServer{instanceKey: "space-a"}
	got, err := s.resolveInstanceKey("")
	if err != nil || got != "space-a" {
		t.Fatalf("unqualified instance = %q, %v", got, err)
	}
	if _, err := s.resolveInstanceKey("space-b"); err == nil {
		t.Fatal("foreign instance was accepted")
	}
}
