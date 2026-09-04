package agent

import (
	"strings"
	"testing"
)

func TestDatasetInUseError_ErrorListsDatasetsWithAgents(t *testing.T) {
	err := &DatasetInUseError{Datasets: []DatasetInUseItem{
		{ID: "kb-1", Agents: []string{"agent-a", "agent-b"}},
		{ID: "kb-2", Agents: []string{"agent-c"}},
	}}
	msg := err.Error()
	for _, want := range []string{
		"请先解除绑定",
		"kb-1（agent-a、agent-b）",
		"kb-2（agent-c）",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message %q missing %q", msg, want)
		}
	}
}
