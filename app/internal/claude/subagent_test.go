package claude

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSubagentSidecars_NewFormat_ReadsMetaAndRecords(t *testing.T) {
	configDir := t.TempDir()
	sessionPath := filepath.Join(configDir, "projects", "D--FixtureProject", "11111111-1111-4111-8111-111111111111.jsonl")
	writeFiles(t, configDir, map[string]string{
		"projects/D--FixtureProject/11111111-1111-4111-8111-111111111111.jsonl": "{}",
		"projects/D--FixtureProject/11111111-1111-4111-8111-111111111111/subagents/agent-aaaa.jsonl": strings.Join([]string{
			`{"type":"user","uuid":"sa-1","isSidechain":true}`,
			`{"type":"assistant","uuid":"sa-2","isSidechain":true}`,
		}, "\n") + "\n",
		"projects/D--FixtureProject/11111111-1111-4111-8111-111111111111/subagents/agent-aaaa.meta.json": `{"agentType":"Explore","description":"find the bug"}`,
	})

	got, err := LoadSubagentSidecars(sessionPath)
	if err != nil {
		t.Fatalf("LoadSubagentSidecars() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1: %+v", len(got), got)
	}
	sa := got[0]
	if sa.AgentID != "agent-aaaa" {
		t.Errorf("AgentID = %q, want %q", sa.AgentID, "agent-aaaa")
	}
	if sa.AgentType != "Explore" {
		t.Errorf("AgentType = %q, want %q", sa.AgentType, "Explore")
	}
	if sa.Description != "find the bug" {
		t.Errorf("Description = %q, want %q", sa.Description, "find the bug")
	}
	if len(sa.Records) != 2 {
		t.Fatalf("len(Records) = %d, want 2", len(sa.Records))
	}
	if sa.Records[0].UUID != "sa-1" || sa.Records[1].UUID != "sa-2" {
		t.Errorf("Records = %+v, want uuids sa-1, sa-2 in order", sa.Records)
	}
}

func TestLoadSubagentSidecars_MultipleAgents_SortedByAgentID(t *testing.T) {
	configDir := t.TempDir()
	sessionPath := filepath.Join(configDir, "projects", "D--FixtureProject", "22222222-2222-4222-8222-222222222222.jsonl")
	writeFiles(t, configDir, map[string]string{
		"projects/D--FixtureProject/22222222-2222-4222-8222-222222222222.jsonl":                          "{}",
		"projects/D--FixtureProject/22222222-2222-4222-8222-222222222222/subagents/agent-zzzz.jsonl":     `{"type":"user","uuid":"z1"}` + "\n",
		"projects/D--FixtureProject/22222222-2222-4222-8222-222222222222/subagents/agent-zzzz.meta.json": `{"agentType":"claude","description":"z"}`,
		"projects/D--FixtureProject/22222222-2222-4222-8222-222222222222/subagents/agent-aaaa.jsonl":     `{"type":"user","uuid":"a1"}` + "\n",
		"projects/D--FixtureProject/22222222-2222-4222-8222-222222222222/subagents/agent-aaaa.meta.json": `{"agentType":"claude","description":"a"}`,
	})

	got, err := LoadSubagentSidecars(sessionPath)
	if err != nil {
		t.Fatalf("LoadSubagentSidecars() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2: %+v", len(got), got)
	}
	if got[0].AgentID != "agent-aaaa" || got[1].AgentID != "agent-zzzz" {
		t.Errorf("order = [%s, %s], want [agent-aaaa, agent-zzzz]", got[0].AgentID, got[1].AgentID)
	}
}

func TestLoadSubagentSidecars_MissingSidecarDir_NoError(t *testing.T) {
	configDir := t.TempDir()
	sessionPath := filepath.Join(configDir, "projects", "D--FixtureProject", "33333333-3333-4333-8333-333333333333.jsonl")
	writeFiles(t, configDir, map[string]string{
		"projects/D--FixtureProject/33333333-3333-4333-8333-333333333333.jsonl": "{}",
	})

	got, err := LoadSubagentSidecars(sessionPath)
	if err != nil {
		t.Fatalf("LoadSubagentSidecars() error = %v, want nil for missing sidecar dir", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want empty", got)
	}
}

func TestLoadSubagentSidecars_CorruptMetaJSON_DegradesToUnknown(t *testing.T) {
	configDir := t.TempDir()
	sessionPath := filepath.Join(configDir, "projects", "D--FixtureProject", "44444444-4444-4444-8444-444444444444.jsonl")
	writeFiles(t, configDir, map[string]string{
		"projects/D--FixtureProject/44444444-4444-4444-8444-444444444444.jsonl":                          "{}",
		"projects/D--FixtureProject/44444444-4444-4444-8444-444444444444/subagents/agent-bbbb.jsonl":     `{"type":"user","uuid":"b1"}` + "\n",
		"projects/D--FixtureProject/44444444-4444-4444-8444-444444444444/subagents/agent-bbbb.meta.json": `not-valid-json{{{`,
	})

	got, err := LoadSubagentSidecars(sessionPath)
	if err != nil {
		t.Fatalf("LoadSubagentSidecars() error = %v, want nil (corrupt meta must degrade, not fail)", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].AgentType != unknownAgentType {
		t.Errorf("AgentType = %q, want %q", got[0].AgentType, unknownAgentType)
	}
	if len(got[0].Records) != 1 {
		t.Errorf("len(Records) = %d, want 1 (jsonl must still load despite bad meta)", len(got[0].Records))
	}
}

func TestLoadSubagentSidecars_MissingMetaJSON_DegradesToUnknown(t *testing.T) {
	configDir := t.TempDir()
	sessionPath := filepath.Join(configDir, "projects", "D--FixtureProject", "55555555-5555-4555-8555-555555555555.jsonl")
	writeFiles(t, configDir, map[string]string{
		"projects/D--FixtureProject/55555555-5555-4555-8555-555555555555.jsonl":                      "{}",
		"projects/D--FixtureProject/55555555-5555-4555-8555-555555555555/subagents/agent-cccc.jsonl": `{"type":"user","uuid":"c1"}` + "\n",
	})

	got, err := LoadSubagentSidecars(sessionPath)
	if err != nil {
		t.Fatalf("LoadSubagentSidecars() error = %v, want nil (missing meta must degrade, not fail)", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].AgentType != unknownAgentType {
		t.Errorf("AgentType = %q, want %q", got[0].AgentType, unknownAgentType)
	}
}

func TestGroupSidechainRecords_GroupsByAgentIDInFirstAppearanceOrder(t *testing.T) {
	records, err := ParseRecords(strings.NewReader(strings.Join([]string{
		`{"type":"user","uuid":"u1"}`,
		`{"type":"user","uuid":"s1","isSidechain":true,"agentId":"task-b"}`,
		`{"type":"assistant","uuid":"s2","isSidechain":true,"agentId":"task-a"}`,
		`{"type":"user","uuid":"s3","isSidechain":true,"agentId":"task-b"}`,
		`{"type":"assistant","uuid":"u2"}`,
	}, "\n") + "\n"))
	if err != nil {
		t.Fatalf("ParseRecords() error = %v", err)
	}

	got := GroupSidechainRecords(records)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2: %+v", len(got), got)
	}
	// task-b appears first (uuid s1), so it must come first despite task-a
	// having a lexicographically smaller AgentID.
	if got[0].AgentID != "task-b" || got[1].AgentID != "task-a" {
		t.Errorf("order = [%s, %s], want [task-b, task-a]", got[0].AgentID, got[1].AgentID)
	}
	if len(got[0].Records) != 2 {
		t.Fatalf("len(task-b Records) = %d, want 2", len(got[0].Records))
	}
	if got[0].Records[0].UUID != "s1" || got[0].Records[1].UUID != "s3" {
		t.Errorf("task-b Records = %+v, want uuids s1, s3 in order", got[0].Records)
	}
	if len(got[1].Records) != 1 || got[1].Records[0].UUID != "s2" {
		t.Errorf("task-a Records = %+v, want single uuid s2", got[1].Records)
	}
}

func TestGroupSidechainRecords_EmptyWhenNoSidechainRecords(t *testing.T) {
	records, err := ParseRecords(strings.NewReader(strings.Join([]string{
		`{"type":"user","uuid":"u1"}`,
		`{"type":"assistant","uuid":"u2"}`,
	}, "\n") + "\n"))
	if err != nil {
		t.Fatalf("ParseRecords() error = %v", err)
	}

	got := GroupSidechainRecords(records)
	if len(got) != 0 {
		t.Errorf("got = %+v, want empty", got)
	}
}

func TestParseRecords_AgentIDField(t *testing.T) {
	records, err := ParseRecords(strings.NewReader(`{"type":"user","uuid":"a","agentId":"task-x"}` + "\n"))
	if err != nil {
		t.Fatalf("ParseRecords() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if records[0].AgentID != "task-x" {
		t.Errorf("AgentID = %q, want %q", records[0].AgentID, "task-x")
	}
}
