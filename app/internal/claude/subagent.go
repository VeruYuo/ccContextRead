package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// unknownAgentType is the AgentType assigned to a sub-agent whose
// .meta.json is missing or fails to parse (T1.5: degrade, don't fail).
const unknownAgentType = "Unknown"

// Subagent is one sub-agent (Task tool) invocation, either loaded from a
// new-format sidecar file pair (<sessionId>/subagents/agent-<id>.jsonl +
// .meta.json) or grouped from old-format isSidechain records inlined in the
// main session file (2.1, T1.5).
type Subagent struct {
	AgentID     string
	AgentType   string
	Description string
	Records     []Record
}

// LoadSubagentSidecars loads new-format sub-agent sidecars that live next
// to the main session file at sessionFilePath, under
// <sessionId>/subagents/. A missing sidecar directory is not an error — it
// yields an empty, nil-error result. A missing or corrupt <agent>.meta.json
// degrades that agent's AgentType to "Unknown" rather than failing the
// whole load. Results are sorted by AgentID for deterministic output.
func LoadSubagentSidecars(sessionFilePath string) ([]Subagent, error) {
	dir := subagentsDir(sessionFilePath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var subagents []Subagent
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "agent-") || !strings.EqualFold(filepath.Ext(name), ".jsonl") {
			continue
		}
		agentID := strings.TrimSuffix(name, filepath.Ext(name))

		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		records, err := ParseRecords(f)
		f.Close()
		if err != nil {
			return nil, err
		}

		agentType, description := readSubagentMeta(filepath.Join(dir, agentID+".meta.json"))
		subagents = append(subagents, Subagent{
			AgentID:     agentID,
			AgentType:   agentType,
			Description: description,
			Records:     records,
		})
	}

	sort.Slice(subagents, func(i, j int) bool { return subagents[i].AgentID < subagents[j].AgentID })
	return subagents, nil
}

// subagentsDir derives the sidecar directory for a main session file path
// <projectsDir>/<encodedDir>/<sessionId>.jsonl, which is
// <projectsDir>/<encodedDir>/<sessionId>/subagents (2.1).
func subagentsDir(sessionFilePath string) string {
	dir := filepath.Dir(sessionFilePath)
	base := filepath.Base(sessionFilePath)
	sessionID := strings.TrimSuffix(base, filepath.Ext(base))
	return filepath.Join(dir, sessionID, "subagents")
}

// readSubagentMeta reads a <agent-id>.meta.json sidecar. Any failure to
// read or parse it degrades to (unknownAgentType, "") instead of
// propagating an error.
func readSubagentMeta(path string) (agentType, description string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return unknownAgentType, ""
	}

	var meta struct {
		AgentType   string `json:"agentType"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return unknownAgentType, ""
	}
	if meta.AgentType == "" {
		return unknownAgentType, meta.Description
	}
	return meta.AgentType, meta.Description
}

// GroupSidechainRecords groups old-format isSidechain:true records inlined
// in a main session file by their AgentID field (T1.5). Non-sidechain
// records are ignored. Groups are returned in order of each AgentID's first
// appearance in records, not sorted, so a Trajectory view can preserve
// chronological ordering of distinct Task invocations.
func GroupSidechainRecords(records []Record) []Subagent {
	var order []string
	groups := map[string][]Record{}

	for _, r := range records {
		if !r.IsSidechain {
			continue
		}
		if _, seen := groups[r.AgentID]; !seen {
			order = append(order, r.AgentID)
		}
		groups[r.AgentID] = append(groups[r.AgentID], r)
	}

	subagents := make([]Subagent, 0, len(order))
	for _, id := range order {
		subagents = append(subagents, Subagent{AgentID: id, Records: groups[id]})
	}
	return subagents
}
