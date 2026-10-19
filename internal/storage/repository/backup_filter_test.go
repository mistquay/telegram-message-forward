package repository

import (
	"encoding/json"
	"testing"

	"gorm.io/datatypes"
)

func TestRemapRequiredFilterIDs(t *testing.T) {
	for _, raw := range []string{`[1,9]`, `[0]`, `[-1]`, `null`, `"1"`, `[1.1]`} {
		if _, err := remapRequiredJSONIDs(datatypes.JSON(raw), map[int64]int64{1: 11}); err == nil {
			t.Fatalf("must not silently remove restrictive reference: %s", raw)
		}
	}
	got, err := remapRequiredJSONIDs(datatypes.JSON(`[1,2]`), map[int64]int64{1: 11, 2: 22})
	if err != nil || string(got) != `[11,22]` {
		t.Fatalf("got=%s err=%v", got, err)
	}
}

func TestRemapFilterBackupPreservesConfigAndGroups(t *testing.T) {
	raw := datatypes.JSON(`{"filter_ids":[3],"conditions":[{"type":"any","negate":true,"children":[{"type":"source","config":{"source_ids":["1"],"custom":true}}]}],"processors":[{"type":"custom","config":{"id":1}}],"extension":42}`)
	got, err := remapFilterNodeConfig(raw, map[int64]int64{1: 10}, map[int64]int64{3: 30})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["extension"] != float64(42) || len(decoded["processors"].([]any)) != 1 {
		t.Fatal("unrelated plugin config must remain intact")
	}
	group := decoded["conditions"].([]any)[0].(map[string]any)
	source := group["children"].([]any)[0].(map[string]any)["config"].(map[string]any)
	if group["negate"] != true || source["source_ids"].([]any)[0] != "10" || source["custom"] != true {
		t.Fatalf("group/source remap lost data: %s", got)
	}
	if _, err := remapFilterNodeConfig(raw, map[int64]int64{}, map[int64]int64{3: 30}); err == nil {
		t.Fatal("missing source restriction must reject restore")
	}
	if _, err := remapFilterNodeConfig(raw, map[int64]int64{1: 10}, map[int64]int64{}); err == nil {
		t.Fatal("missing shared filter must reject restore")
	}
}
