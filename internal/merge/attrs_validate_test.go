package merge

import (
	"fmt"
	"strings"
	"testing"
)

func TestValidateAttributes_TruncationKeepsClusterAndRole(t *testing.T) {
	attrs := map[string]string{
		"cluster": "prod",
		"role":    "gateway",
	}
	// 64 keys that sort alphabetically before "cluster"/"role", simulating an
	// agent that reports far more custom attributes than admin labels ever
	// could.
	for i := 0; i < maxAttributeKeys; i++ {
		attrs[fmt.Sprintf("a%02d", i)] = "v"
	}

	out := ValidateAttributes(attrs)

	if len(out) != maxAttributeKeys {
		t.Fatalf("expected exactly %d keys kept, got %d", maxAttributeKeys, len(out))
	}
	if out["cluster"] != "prod" {
		t.Errorf("expected cluster to survive truncation, got %q", out["cluster"])
	}
	if out["role"] != "gateway" {
		t.Errorf("expected role to survive truncation, got %q", out["role"])
	}
}

func TestValidateAttributes_MixedCaseClusterRoleSurviveTruncation(t *testing.T) {
	attrs := map[string]string{
		"Cluster": "prod",
		"ROLE":    "gateway",
	}
	for i := 0; i < maxAttributeKeys; i++ {
		attrs[fmt.Sprintf("a%02d", i)] = "v"
	}

	out := ValidateAttributes(attrs)

	if _, ok := out["cluster"]; !ok {
		t.Error("expected lowercased cluster to survive truncation")
	}
	if _, ok := out["role"]; !ok {
		t.Error("expected lowercased role to survive truncation")
	}
}

func TestValidateAttributes_CapsAtMaxKeys(t *testing.T) {
	attrs := make(map[string]string, maxAttributeKeys+10)
	for i := 0; i < maxAttributeKeys+10; i++ {
		attrs[fmt.Sprintf("key%03d", i)] = "v"
	}

	out := ValidateAttributes(attrs)

	if len(out) != maxAttributeKeys {
		t.Fatalf("expected exactly %d keys kept, got %d", maxAttributeKeys, len(out))
	}
}

func TestValidateAttributes_DropsInvalidKeyOrValue(t *testing.T) {
	attrs := map[string]string{
		"team":        "platform",
		"bad key":     "v",
		"empty-value": "",
		strings.Repeat("k", maxAttributeKeyLength+1): "v",
		"too-long-value": strings.Repeat("v", maxAttributeValueLength+1),
	}

	out := ValidateAttributes(attrs)

	if len(out) != 1 || out["team"] != "platform" {
		t.Errorf("expected only 'team' to survive, got %v", out)
	}
}

func TestValidateAttributes_LowercasesKeys(t *testing.T) {
	out := ValidateAttributes(map[string]string{"Team": "platform"})
	if out["team"] != "platform" {
		t.Errorf("expected key to be lowercased, got %v", out)
	}
}

func TestValidateAttributes_KeepsReservedKeys(t *testing.T) {
	// Unlike admin labels, reserved keys reported by an agent (e.g.
	// collector.version) must still be stored/displayed -- IsReserved is
	// enforced at merge time, not here.
	out := ValidateAttributes(map[string]string{"collector.version": "v1.19.2"})
	if out["collector.version"] != "v1.19.2" {
		t.Errorf("expected reserved key to be kept, got %v", out)
	}
}

func TestValidateAttributes_TruncationKeepsAllReservedKeys(t *testing.T) {
	// Truncation priority is tied to IsReserved directly (not a second,
	// hand-maintained list), so any reserved key -- not just cluster/role --
	// survives the cap ahead of arbitrary custom attributes.
	attrs := map[string]string{
		"id":                "c1",
		"os":                "linux",
		"alloy_version":     "v1.19.2",
		"collector.version": "v1.19.2",
	}
	for i := 0; i < maxAttributeKeys; i++ {
		attrs[fmt.Sprintf("a%02d", i)] = "v"
	}

	out := ValidateAttributes(attrs)

	for _, reserved := range []string{"id", "os", "alloy_version", "collector.version"} {
		if _, ok := out[reserved]; !ok {
			t.Errorf("expected reserved key %q to survive truncation, got %v", reserved, out)
		}
	}
}

func TestValidateAttributes_EmptyInput(t *testing.T) {
	if out := ValidateAttributes(map[string]string{}); len(out) != 0 {
		t.Errorf("expected empty map, got %v", out)
	}
	if out := ValidateAttributes(nil); out != nil {
		t.Errorf("expected nil passthrough, got %v", out)
	}
}
