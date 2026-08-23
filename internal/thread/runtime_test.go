package thread

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The set is closed at selection, and admission is a second gate inside each
// adapter: Pi has decision 0052's evidence and the other two do not, so
// selecting one of those still refuses after the configuration checks pass.
func TestRuntimeSelectionIsClosedAndAdmittedPerAdapter(t *testing.T) {
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
		{"pi", string(Pi), Pi, ""},
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
				cfg = claudeConfig(t)
			}
			if tc.runtime == Codex {
				cfg = codexConfig(t)
			}
			if tc.runtime == Pi {
				cfg = piConfig(t)
			}
			_, err = adapter.Prepare(cfg, packet, adapterReceipt)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("the admitted %s adapter refused: %v", tc.runtime, err)
				}
				return
			}
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
	credentialed := func(edit func(*RuntimeConfig)) RuntimeConfig {
		cfg := claudeConfig(t)
		edit(&cfg)
		return cfg
	}
	for _, tc := range []struct {
		config RuntimeConfig
		want   string
	}{
		{RuntimeConfig{Runtime: "claude", Version: "pending"}, "executable is unset"},
		{RuntimeConfig{Runtime: "claude", Executable: "claude", Version: "pending"}, "explicit absolute path"},
		{RuntimeConfig{Runtime: "claude", Executable: "/configured/claude"}, "version is unset"},
		{credentialed(func(cfg *RuntimeConfig) { cfg.Runtime = "codex" }), "does not name claude"},
		{credentialed(func(cfg *RuntimeConfig) { cfg.Version = "pending" }), "version is unsupported"},
		{credentialed(func(cfg *RuntimeConfig) { cfg.ClaudeSetupToken = "" }), "CLAUDE_CODE_OAUTH_TOKEN is unset"},
	} {
		if _, err := adapter.Prepare(tc.config, packet, nil); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Prepare(%+v) = %v, want refusal %q", tc.config, err, tc.want)
		}
	}
	if _, err := adapter.Prepare(claudeConfig(t), Packet{}, nil); err == nil || !strings.Contains(err.Error(), "packet is absent") {
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
	err = Dispatch(context.Background(), codexConfig(t), packet, launch, Supervisor{Timeout: time.Second}, receipt)
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

// heldInput is a bound input channel a test can inspect. The launch hands an
// adapter a WriteCloser because the adapter decides when the session ends, so a
// test that drives BindInput has to supply one and has to be able to say
// whether it was closed.
type heldInput struct{ *bytes.Buffer }

func (heldInput) Close() error { return nil }

// countedInput records the closes as well as the bytes, which is what a test
// about who ends the session needs.
type countedInput struct {
	bytes.Buffer
	closes int
}

func (c *countedInput) Close() error {
	c.closes++
	return nil
}
