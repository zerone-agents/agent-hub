package handler

import (
	"encoding/json"
	"strings"
)

// runtimeActivity is the normalized, provider-neutral execution trace extracted
// from Agent Runtime's SSE protocol. It is deliberately not a platform event:
// H1 stores it as append-only Run activity, while H2 may later publish selected
// activities through the unified event envelope.
type runtimeActivity struct {
	Kind       string
	StepID     string
	Name       string
	Status     string
	InputJSON  string
	OutputJSON string
	Error      string
}

// extractRuntimeActivities turns an SDK-specific SSE transcript into the small
// vocabulary understood by Run. Unknown and malformed events are ignored so a
// runtime upgrade cannot make chat persistence fail.
func extractRuntimeActivities(sse string) []runtimeActivity {
	type toolUse struct {
		Type  string      `json:"type"`
		ID    string      `json:"id"`
		Name  string      `json:"name"`
		Input interface{} `json:"input"`
	}

	activities := make([]runtimeActivity, 0)
	eventName := ""
	for _, rawLine := range strings.Split(sse, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			eventName = ""
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "{}" {
			continue
		}

		switch eventName {
		case "assistant":
			var envelope struct {
				Message struct {
					Content []toolUse `json:"content"`
				} `json:"message"`
			}
			if json.Unmarshal([]byte(payload), &envelope) != nil {
				continue
			}
			for _, block := range envelope.Message.Content {
				if block.Type != "tool_use" {
					continue
				}
				activities = append(activities, runtimeActivity{
					Kind:      "tool_started",
					StepID:    block.ID,
					Name:      block.Name,
					Status:    "running",
					InputJSON: marshalActivityValue(block.Input),
				})
			}
		case "tool_result":
			var envelope struct {
				Result struct {
					ToolUseID string      `json:"tool_use_id"`
					ToolName  string      `json:"tool_name"`
					Output    interface{} `json:"output"`
					IsError   bool        `json:"is_error"`
				} `json:"result"`
			}
			if json.Unmarshal([]byte(payload), &envelope) != nil {
				continue
			}
			status := "completed"
			kind := "tool_finished"
			if envelope.Result.IsError {
				status = "failed"
			}
			activities = append(activities, runtimeActivity{
				Kind:       kind,
				StepID:     envelope.Result.ToolUseID,
				Name:       envelope.Result.ToolName,
				Status:     status,
				OutputJSON: marshalActivityValue(envelope.Result.Output),
			})
		case "result":
			var envelope struct {
				Type      string   `json:"type"`
				Subtype   string   `json:"subtype"`
				ErrorType string   `json:"error_type"`
				Errors    []string `json:"errors"`
			}
			if json.Unmarshal([]byte(payload), &envelope) != nil || envelope.Type != "result" {
				continue
			}
			if envelope.Subtype == "error" {
				errText := strings.Join(envelope.Errors, "\n")
				if errText == "" {
					errText = envelope.ErrorType
				}
				activities = append(activities, runtimeActivity{Kind: "failed", Status: "failed", Error: errText})
			} else if envelope.Subtype == "success" {
				activities = append(activities, runtimeActivity{Kind: "completed", Status: "completed"})
			}
		}
	}
	return activities
}

func marshalActivityValue(value interface{}) string {
	if value == nil {
		return ""
	}
	b, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(b)
}
