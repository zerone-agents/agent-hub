package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

type AgentChatHandler struct {
	svc *services.AgentChatService
}

func NewAgentChatHandler(svc *services.AgentChatService) *AgentChatHandler {
	return &AgentChatHandler{svc: svc}
}

func (h *AgentChatHandler) ListSessions(c *gin.Context) {
	agentName := services.NormalizeAgentName(c.Param("name"))
	userID := c.MustGet("user_id").(string)
	source := c.Query("source")

	page, pageSize := parsePagination(c, 1, 30)
	sessions, total, err := h.svc.ListSessions(tenant.GetTenantID(c), userID, agentName, source, page, pageSize)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondSuccess(c, gin.H{
		"items": sessions,
		"total": total,
	})
}

type createSessionReq struct {
	Title string `json:"title"`
}

func (h *AgentChatHandler) CreateSession(c *gin.Context) {
	agentName := services.NormalizeAgentName(c.Param("name"))
	userID := c.MustGet("user_id").(string)
	userName, _ := c.Get("user_name")
	displayName, _ := c.Get("display_name")

	var req createSessionReq
	_ = c.ShouldBindJSON(&req)

	sess, err := h.svc.CreateSession(
		tenant.GetTenantID(c),
		userID,
		agentName,
		req.Title,
		stringOrEmpty(userName),
		stringOrEmpty(displayName),
	)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondSuccess(c, sess)
}

func (h *AgentChatHandler) ListMessages(c *gin.Context) {
	sessionID := c.Param("id")
	userID := c.MustGet("user_id").(string)

	page, pageSize := parsePagination(c, 1, 50)
	msgs, total, err := h.svc.GetMessages(tenant.GetTenantID(c), userID, sessionID, page, pageSize)
	if err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	respondSuccess(c, gin.H{
		"items": msgs,
		"total": total,
	})
}

func (h *AgentChatHandler) DeleteSession(c *gin.Context) {
	sessionID := c.Param("id")
	userID := c.MustGet("user_id").(string)

	if err := h.svc.DeleteSession(tenant.GetTenantID(c), userID, sessionID); err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	respondMessage(c, http.StatusOK, "session deleted")
}

type sendMessageReq struct {
	Content string `json:"content" binding:"required"`
}

// SendMessage is the SSE streaming endpoint.
func (h *AgentChatHandler) SendMessage(c *gin.Context) {
	agentName := services.NormalizeAgentName(c.Param("name"))
	sessionID := c.Param("id")
	userID := c.MustGet("user_id").(string)
	tenantID := tenant.GetTenantID(c)

	var req sendMessageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "content is required")
		return
	}

	// 1. Persist user message and load session metadata
	if _, err := h.svc.SaveUserMessage(tenantID, userID, sessionID, req.Content); err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	// Name a new session after the first user message (truncated to 20 chars).
	_ = h.svc.AutoTitleSession(tenantID, sessionID, req.Content)

	sess, err := h.svc.GetSession(tenantID, userID, sessionID)
	if err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}

	// 2. Resolve runtime URL and API key
	baseURL, apiKey, err := h.svc.ResolveRuntime(tenantID, agentName)
	if err != nil {
		h.saveErrorMessage(tenantID, userID, sessionID, "Agent 暂不可用："+err.Error())
		respondError(c, http.StatusConflict, "agent not available: "+err.Error())
		return
	}

	// 3. Build runtime request body. Re-use the runtime SDK session id if this
	// control-panel session has already been bound to one.
	body := map[string]string{"message": req.Content}
	if sess.RuntimeSessionID != "" {
		body["sessionId"] = sess.RuntimeSessionID
	}
	bodyBytes, _ := json.Marshal(body)

	// 4. Open runtime stream
	ctx := c.Request.Context()
	rc, err := h.svc.RuntimeClient().StreamRun(ctx, baseURL, agentName, apiKey, bodyBytes)
	if err != nil {
		h.saveErrorMessage(tenantID, userID, sessionID, "Runtime 连接失败："+err.Error())
		respondError(c, http.StatusBadGateway, "runtime unreachable: "+err.Error())
		return
	}
	defer rc.Close()

	// 5. SSE headers + flusher
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		respondError(c, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	flusher.Flush()

	// 6. Pipe byte-for-byte, buffer for aggregation. A heartbeat goroutine
	// emits SSE comments (": ping\n\n") every 15s to keep intermediate proxies
	// (nginx/Kong/Cloudflare) from killing the connection during long tool
	// executions, where the runtime stream is otherwise silent for minutes.
	// The writer is shared between the pump loop and the heartbeat goroutine,
	// so both are serialized through writeMu.
	var writeMu sync.Mutex
	heartStop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-heartStop:
				return
			case <-ticker.C:
				writeMu.Lock()
				_, _ = c.Writer.Write([]byte(": ping\n\n"))
				flusher.Flush()
				writeMu.Unlock()
			}
		}
	}()

	var aggregate strings.Builder
	scanner := bufio.NewScanner(rc)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		writeMu.Lock()
		_, _ = c.Writer.Write(line)
		_, _ = c.Writer.Write([]byte("\n"))
		flusher.Flush()
		writeMu.Unlock()

		aggregate.Write(line)
		aggregate.WriteByte('\n')
	}
	close(heartStop)

	// 7. Persist whatever we captured — even if the client disconnected.
	// Previously this branch short-circuited on ctx.Err(), so a proxy timeout
	// during a long tool call erased the entire assistant turn and a refresh
	// showed nothing. Now we always flush buffered content to the DB; the
	// runtime keeps executing regardless of the downstream connection.
	aggregateStr := aggregate.String()

	// result/subtype=error（如 429 配额耗尽）是正常结束的流里携带的运行时
	// 失败，scanner.Err 为 nil 覆盖不到；落库为系统错误消息，刷新后历史可见。
	// 与流中断分支互斥，避免同一次失败双重落库。
	if runErr := extractRuntimeError(aggregateStr); runErr != "" {
		h.saveErrorMessage(tenantID, userID, sessionID, runErr)
	} else if scanErr := scanner.Err(); scanErr != nil {
		// scanner.Err() != nil means the runtime stream was truncated (network
		// error, runtime crash, ctx cancellation, etc.). Surface it to the user so
		// a refresh shows that the turn was interrupted.
		h.saveErrorMessage(tenantID, userID, sessionID, "Runtime 流中断："+scanErr.Error())
	}

	// 8. Bind the runtime SDK session id returned on the first run. The runtime
	// emits it in a system.init event; subsequent messages must include it to
	// preserve context.
	if sess.RuntimeSessionID == "" {
		if runtimeSID := extractRuntimeSessionID(aggregateStr); runtimeSID != "" {
			_ = h.svc.BindRuntimeSessionID(tenantID, sessionID, runtimeSID)
		}
	}

	contentJSON := aggregateSSEToContent(aggregateStr)
	if contentJSON != "" {
		aigcLabel := extractAigcLabel(aggregateStr)
		_, _ = h.svc.SaveAssistantMessage(tenantID, userID, sessionID, contentJSON, aigcLabel)
	}
}

// saveErrorMessage persists a system error message so the failure is visible
// in the chat history after a refresh.
func (h *AgentChatHandler) saveErrorMessage(tenantID, userID, sessionID, message string) {
	payload, _ := json.Marshal([]map[string]string{{"type": "error", "message": message}})
	_, _ = h.svc.SaveSystemMessage(tenantID, userID, sessionID, string(payload))
}

// extractRuntimeError scans the buffered SSE stream for a result event with
// subtype=error (e.g. 429 quota exhausted, surfaced after runtime-internal
// retries give up) and returns the joined error messages, falling back to
// error_type when the errors array is empty. Returns "" when the run did not
// fail. Malformed lines are skipped.
func extractRuntimeError(sse string) string {
	lines := strings.Split(sse, "\n")
	var eventName string
	var messages []string
	var errorType string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			eventName = ""
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if eventName != "result" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		var data struct {
			Type      string   `json:"type"`
			Subtype   string   `json:"subtype"`
			ErrorType string   `json:"error_type"`
			Errors    []string `json:"errors"`
		}
		if err := json.Unmarshal([]byte(payload), &data); err != nil {
			continue
		}
		if data.Type == "result" && data.Subtype == "error" {
			messages = append(messages, data.Errors...)
			if errorType == "" {
				errorType = data.ErrorType
			}
		}
	}
	if len(messages) > 0 {
		return strings.Join(messages, "\n")
	}
	return errorType
}

// extractRuntimeSessionID scans the raw SSE stream for the first system.init
// event and returns its session_id field.
func extractRuntimeSessionID(sse string) string {
	lines := strings.Split(sse, "\n")
	var eventName string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			eventName = ""
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if eventName != "system" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		var data struct {
			Type      string `json:"type"`
			Subtype   string `json:"subtype"`
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal([]byte(payload), &data); err != nil {
			continue
		}
		if data.Type == "system" && data.Subtype == "init" && data.SessionID != "" {
			return data.SessionID
		}
	}
	return ""
}

// aggregateSSEToContent parses buffered SSE stream into a content JSON array.
func aggregateSSEToContent(sse string) string {
	type segment struct {
		Type      string            `json:"type"`
		Text      string            `json:"text,omitempty"`
		Reasoning string            `json:"reasoning,omitempty"`
		Name      string            `json:"name,omitempty"`
		ID        string            `json:"id,omitempty"`
		Input     map[string]string `json:"input,omitempty"`
		Content   interface{}       `json:"content,omitempty"`
		ToolUseID string            `json:"toolUseId,omitempty"`
		IsError   bool              `json:"isError,omitempty"`
	}

	// SDK SSE 协议（2026-07-06 验证）：
	//   partial_message × N → assistant × 1 → [tool_result → partial_message × M → assistant × 1]...
	// 后端持久化只需 assistant（turn 完整 message）和 tool_result，忽略 partial_message
	// （流式增量不需要持久化，assistant 已包含完整内容）。按事件顺序追加到单个 slice。
	var segments []segment

	lines := strings.Split(sse, "\n")
	var eventName string
	for _, line := range lines {
		line = strings.TrimSpace(line)
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

		var generic map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &generic); err != nil {
			continue
		}

		switch eventName {
		case "assistant":
			// turn 完成的完整 message，按事件顺序直接追加
			msg, _ := generic["message"].(map[string]interface{})
			if msg == nil {
				continue
			}
			contentArr, _ := msg["content"].([]interface{})
			for _, item := range contentArr {
				block, _ := item.(map[string]interface{})
				if block == nil {
					continue
				}
				blockType, _ := block["type"].(string)
				switch blockType {
				case "text":
					if text, _ := block["text"].(string); text != "" {
						segments = append(segments, segment{Type: "text", Text: text})
					}
				case "thinking":
					if thinking, _ := block["thinking"].(string); thinking != "" {
						segments = append(segments, segment{Type: "reasoning", Reasoning: thinking})
					}
				case "tool_use":
					name, _ := block["name"].(string)
					id, _ := block["id"].(string)
					input, _ := block["input"].(map[string]interface{})
					strInput := make(map[string]string, len(input))
					for k, v := range input {
						strInput[k] = fmt.Sprintf("%v", v)
					}
					segments = append(segments, segment{Type: "tool_use", Name: name, ID: id, Input: strInput})
				}
			}
		case "tool_result":
			// SDK 协议：result 是嵌套对象 {tool_use_id, tool_name, output}
			resultObj, _ := generic["result"].(map[string]interface{})
			var content interface{}
			var toolUseID string
			if resultObj != nil {
				content = resultObj["output"]
				if id, ok := resultObj["tool_use_id"].(string); ok {
					toolUseID = id
				}
			}
			segments = append(segments, segment{
				Type:      "tool_result",
				Content:   content,
				ToolUseID: toolUseID,
				// SDK 当前不在 SSE 事件里暴露 is_error，固定 false
			})
		}
	}

	if len(segments) == 0 {
		return ""
	}
	b, err := json.Marshal(segments)
	if err != nil {
		return ""
	}
	return string(b)
}

// extractAigcLabel scans the raw SSE stream for the GB 45438-2025 label the
// runtime injects into both the system init event and the result event
// (dual anchors for truncated streams). The result event is authoritative;
// the first system-event value is the fallback. Returns the raw aigc JSON
// object, or "" when no label was present. Malformed lines are skipped.
func extractAigcLabel(sse string) string {
	lines := strings.Split(sse, "\n")
	var eventName string
	var fallback, authoritative string
	for _, line := range lines {
		line = strings.TrimSpace(line)
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
		if eventName != "system" && eventName != "result" {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		var data struct {
			Aigc json.RawMessage `json:"aigc"`
		}
		if err := json.Unmarshal([]byte(payload), &data); err != nil {
			continue
		}
		if len(data.Aigc) == 0 || string(data.Aigc) == "null" {
			continue
		}
		switch eventName {
		case "result":
			authoritative = string(data.Aigc)
		case "system":
			if fallback == "" {
				fallback = string(data.Aigc)
			}
		}
	}
	if authoritative != "" {
		return authoritative
	}
	return fallback
}
