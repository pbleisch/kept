package thread

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRuntimeSelectionIsClosedAndUnadmitted(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   string
		runtime Runtime
		want    string
	}{
		{"unset", "", "", "runtime is unset"},
		{"unknown", "other", "", "runtime is unknown"},
		{"claude", string(Claude), Claude, "runtime is unadmitted"},
		{"codex", string(Codex), Codex, "runtime is unadmitted"},
		{"pi", string(Pi), Pi, "runtime is unadmitted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter, err := Select(tc.value)
			if tc.runtime == "" {
				if adapter != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("Select(%q) = %T, %v; want refusal %q", tc.value, adapter, err, tc.want)
				}
				return
			}
			if err != nil || adapter.Runtime() != tc.runtime {
				t.Fatalf("Select(%q) = %T, %v; want the static %s adapter", tc.value, adapter, err, tc.runtime)
			}
			packet, err := AssemblePacket(packetInput())
			if err != nil {
				t.Fatal(err)
			}
			receipt, _ := testReceipt(t)
			adapterReceipt, err := receipt.ForAdapter(adapter.Runtime(), adapter.AdapterVersion())
			if err != nil {
				t.Fatal(err)
			}
			cfg := RuntimeConfig{Runtime: tc.value, Executable: "/configured/runtime", Version: "evidence-pending"}
			if tc.runtime == Claude {
				cfg.Version = ClaudeVersion
				cfg.AnthropicAPIKey = "test-key"
			}
			if tc.runtime == Pi {
				cfg = piConfig(t)
			}
			_, err = adapter.Prepare(cfg, packet, adapterReceipt)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("%s Prepare = %v, want refusal %q", tc.runtime, err, tc.want)
			}
		})
	}
}

func TestAnAdapterNeverDiscoversPathOrAcceptsAnIncompleteConfiguration(t *testing.T) {
	adapter, err := Select(string(Claude))
	if err != nil {
		t.Fatal(err)
	}
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		config RuntimeConfig
		want   string
	}{
		{RuntimeConfig{Runtime: "claude", Version: "pending"}, "executable is unset"},
		{RuntimeConfig{Runtime: "claude", Executable: "claude", Version: "pending"}, "explicit absolute path"},
		{RuntimeConfig{Runtime: "claude", Executable: "/configured/claude"}, "version is unset"},
		{RuntimeConfig{Runtime: "codex", Executable: "/configured/claude", Version: ClaudeVersion, AnthropicAPIKey: "test-key"}, "does not name claude"},
		{RuntimeConfig{Runtime: "claude", Executable: "/configured/claude", Version: "pending", AnthropicAPIKey: "test-key"}, "version is unsupported"},
		{RuntimeConfig{Runtime: "claude", Executable: "/configured/claude", Version: ClaudeVersion}, "ANTHROPIC_API_KEY is unset"},
	} {
		if _, err := adapter.Prepare(tc.config, packet, nil); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Prepare(%+v) = %v, want refusal %q", tc.config, err, tc.want)
		}
	}
	if _, err := adapter.Prepare(RuntimeConfig{
		Runtime: string(Claude), Executable: "/configured/claude", Version: ClaudeVersion, AnthropicAPIKey: "test-key",
	}, Packet{}, nil); err == nil || !strings.Contains(err.Error(), "packet is absent") {
		t.Fatalf("Prepare with no packet = %v, want refusal", err)
	}
}

func TestAdapterReceiptsCannotWriteParentLifecycle(t *testing.T) {
	typ := reflect.TypeOf((*AdapterReceipt)(nil)).Elem()
	for _, method := range []string{"Started", "TimedOut", "Killed", "Exited"} {
		if _, found := typ.MethodByName(method); found {
			t.Errorf("AdapterReceipt exposes parent-owned lifecycle method %s", method)
		}
	}
}

func TestDispatchRefusesBeforeAnUnadmittedRuntimeCanLaunch(t *testing.T) {
	receipt, _ := testReceipt(t)
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	launch := &fakeLaunch{child: normalChild(0)}
	err = Dispatch(context.Background(), RuntimeConfig{
		Runtime: string(Codex), Executable: "/configured/codex", Version: "pending",
	}, packet, launch, Supervisor{Timeout: time.Second}, receipt)
	if err == nil || !strings.Contains(err.Error(), "unadmitted") {
		t.Fatalf("Dispatch = %v, want an unadmitted-runtime refusal", err)
	}
	if launch.started != 0 {
		t.Fatalf("unadmitted Codex launched %d children", launch.started)
	}
}

func TestDispatchRefusesOnAFailedCanaryBeforeSelectingARuntime(t *testing.T) {
	receipt, _ := testReceipt(t)
	packet, err := AssemblePacket(packetInput())
	if err != nil {
		t.Fatal(err)
	}
	launch := &fakeLaunch{verified: errors.New("escape probe succeeded")}
	err = Dispatch(context.Background(), RuntimeConfig{Runtime: "unknown"}, packet, launch, Supervisor{Timeout: time.Second}, receipt)
	if err == nil || !strings.Contains(err.Error(), "canary failed") {
		t.Fatalf("Dispatch = %v, want a failed-canary refusal", err)
	}
	if launch.started != 0 {
		t.Fatal("a failed confinement canary started a runtime")
	}
}
