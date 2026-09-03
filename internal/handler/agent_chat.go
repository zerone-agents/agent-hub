package handler

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/chat"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/infrastructure/runtime"

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

// Capabilities reports feature availability for the agent chat page
// (issue #94: attachments require runtime >= 2.5.0).
func (h *AgentChatHandler) Capabilities(c *gin.Context) {
	agentName := services.NormalizeAgentName(c.Param("name"))
	ok := h.svc.AttachmentsAvailable(c.Request.Context(), tenant.GetTenantID(c), agentName)
	respondSuccess(c, gin.H{"attachmentsEnabled": ok})
}

type sendMessageReq struct {
	Content     string                    `json:"content"`
	Attachments []services.AttachmentDesc `json:"attachments"`
}

// runRequestBody is the POST /v1/agents/{key}/runs JSON body. attachments
// are relayed verbatim (runtime re-validates name/size/path).
type runRequestBody struct {
	Message     string                    `json:"message"`
	SessionID   string                    `json:"sessionId,omitempty"`
	Attachments []services.AttachmentDesc `json:"attachments,omitempty"`
}

// SendMessage is the SSE streaming endpoint.
func (h *AgentChatHandler) SendMessage(c *gin.Context) {
	agentName := services.NormalizeAgentName(c.Param("name"))
	sessionID := c.Param("id")
	userID := c.MustGet("user_id").(string)
	tenantID := tenant.GetTenantID(c)

	var req sendMessageReq
	// 严格解码（issue #94 review F4）：未知字段（如伪造的 base64 内联数据）
	// 一律 400，不静默丢弃——客户端契约错误尽早暴露。
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		respondErrorCode(c, http.StatusBadRequest, chat.ErrCodeInvalidAttachment, "请求包含无法识别的字段")
		return
	}
	// 尾随值（issue #94 review R2 F2）：首个 JSON 值之后必须直接 EOF——
	// `{"content":"hi"}{"base64":...}` 这类拼接 body 一律 400，不静默取首值。
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		respondErrorCode(c, http.StatusBadRequest, chat.ErrCodeInvalidAttachment, "请求格式错误")
		return
	}
	if strings.TrimSpace(req.Content) == "" && len(req.Attachments) == 0 {
		respondErrorCode(c, http.StatusBadRequest, chat.ErrCodeInvalidAttachment, "请输入文本或添加附件")
		return
	}
	if err := services.ValidateAttachmentDescs(req.Attachments); err != nil {
		log.Printf("[chat] rejected attachment descriptors: session=%s err=%v", sessionID, err)
		respondErrorCode(c, http.StatusBadRequest, chat.ErrCodeInvalidAttachment, "附件信息无效")
		return
	}

	// 1. Load session FIRST and verify the agent binding before persisting
	// anything (issue #94: session.AgentID must match :name — a mismatch
	// 404s without leaking existence).
	sess, err := h.svc.GetSession(tenantID, userID, sessionID)
	if err != nil {
		log.Printf("[chat] send message session lookup failed: tenant=%s session=%s user=%s err=%v",
			tenantID, sessionID, userID, err)
		respondError(c, http.StatusNotFound, "会话不存在")
		return
	}
	if sess.AgentID != agentName {
		respondError(c, http.StatusNotFound, "会话不存在")
		return
	}
	// 2. Resolve runtime URL, API key, and the deployer-reported container id
	// BEFORE anything is persisted: the attachment probe below needs the base
	// URL, and a resolve failure must not leave a persisted user turn (or an
	// orphan system error message) behind — the client keeps its input and
	// can simply retry.
	baseURL, apiKey, containerID, err := h.svc.ResolveRuntime(tenantID, agentName)
	if err != nil {
		// HTTP 响应只给中性中文文案，英文细节进日志（CONTRIBUTING Standards 1）。
		log.Printf("[chat] resolve runtime failed: tenant=%s agent=%s session=%s err=%v",
			tenantID, agentName, sessionID, err)
		respondError(c, http.StatusConflict, "Agent 暂不可用，请稍后重试")
		return
	}

	// 空 containerID fail-closed（issue #94 review R4 P1-2）：deployer 未报告
	// 容器代次 ⇒ 代次授权锚点缺失。若放行到记录校验，「历史空代记录 + 当前
	// 空 ID」会判相等通过（fail-open）——入口统一拒绝；纯文本消息不受影响。
	if len(req.Attachments) > 0 && containerID == "" {
		log.Printf("[chat] send rejected, empty container generation: tenant=%s agent=%s session=%s",
			tenantID, agentName, sessionID)
		respondError(c, http.StatusServiceUnavailable, "部署状态异常，附件暂不可用，请稍后重试")
		return
	}

	// 3. 附件能力门控（issue #94 review F3）：/health 版本探测。旧 runtime
	//（< 2.5.0）对 run 请求的 attachments 字段静默忽略——宁可拒绝也不静默
	// 丢附件。探测先于持久化：拒绝时还没有任何已落库内容需要回滚。纯文本
	// 消息跳过探测。
	if len(req.Attachments) > 0 {
		if !h.svc.AttachmentsSupportedAt(c.Request.Context(), baseURL) {
			respondErrorCode(c, http.StatusNotImplemented, chat.ErrCodeRuntimeAttachmentUnsupported,
				"当前 Runtime 版本不支持附件（需 ≥ 2.5.0），请升级 Runtime 并重新部署 Agent")
			return
		}
	}

	// 4. Upload-record binding (issue #94 review F1 + R3)：每个描述符必须
	// 与服务端在上传时落库的记录全等，且记录必须属于当前容器代次——
	// deployer 报告的不可变 container id 精确相等（零时间容差）。重部署
	// 清空 `.zerone-uploads` 后，同名文件的新上传会拿到与旧容器完全相同的
	// 路径，旧代记录不能再授权（否则读到的是新上传者的字节）。`.zerone-uploads`
	// 是同 Agent runtime 容器内所有用户共享的目录——仅做语法校验就落库的
	// 话，伪造描述符即可把他人 path 写进自己会话再经内容代理下载。
	// rec.ContainerID == "" 恒拒（review R4 P1-2）：升级前落库的历史空代
	// 记录一律无效——空对空判相等的放行口子从记录侧也封死。
	for _, a := range req.Attachments {
		rec, err := h.svc.GetUploadRecord(tenantID, userID, sessionID, a.ID)
		if err != nil || rec.Name != a.Name || rec.Mime != a.Mime || rec.Size != a.Size || rec.Path != a.Path ||
			rec.ContainerID == "" || rec.ContainerID != containerID {
			respondErrorCode(c, http.StatusBadRequest, chat.ErrCodeInvalidAttachment,
				"附件信息无效或已失效，请重新上传")
			return
		}
	}
	msg, err := h.svc.SaveUserMessage(tenantID, userID, sessionID, req.Content, req.Attachments)
	if err != nil {
		log.Printf("[chat] save user message failed: tenant=%s session=%s user=%s err=%v",
			tenantID, sessionID, userID, err)
		respondError(c, http.StatusNotFound, "会话不存在")
		return
	}
	// 会话标题：优先文本；纯附件消息取第一个文件名（issue #94）。
	titleSeed := req.Content
	if titleSeed == "" && len(req.Attachments) > 0 {
		titleSeed = req.Attachments[0].Name
	}
	_ = h.svc.AutoTitleSession(tenantID, sessionID, titleSeed)

	// 5. Build runtime request body. Re-use the runtime SDK session id if this
	// control-panel session has already been bound to one.
	body := runRequestBody{Message: req.Content}
	if sess.RuntimeSessionID != "" {
		body.SessionID = sess.RuntimeSessionID
	}
	if len(req.Attachments) > 0 {
		body.Attachments = req.Attachments
	}
	bodyBytes, _ := json.Marshal(body)

	// 6. Open runtime stream
	ctx := c.Request.Context()
	// runtime 注册名为裸 Agent ID（issue #114）；scoped deployment key 仅是
	// deployer 资源标识，不参与 runtime 寻址。
	rc, err := h.svc.RuntimeClient().StreamRun(ctx, baseURL, services.NormalizeAgentName(agentName), apiKey, bodyBytes)
	if err != nil {
		// Runtime run-attachment domain errors (attachment_missing etc.) are
		// pre-run failures: surface the code so the frontend can retry from
		// local files instead of persisting a system error message.
		var httpErr *runtime.RuntimeHTTPError
		if errors.As(err, &httpErr) {
			if code, ok := runtimeAttachmentCode(httpErr.Body); ok {
				// Run 前失败（attachment_missing 等）：回滚乐观持久化的用户消息，
				// 重发（重新上传→再发）才不会在历史里留下重复的 user turn。
				// 删除失败仅日志——消息重复是体验问题，不该阻塞错误上报。
				if delErr := h.svc.DeleteMessageByID(tenantID, userID, sessionID, msg.ID); delErr != nil {
					log.Printf("[chat] rollback user message failed: tenant=%s session=%s msg=%s err=%v",
						tenantID, sessionID, msg.ID, delErr)
				}
				log.Printf("[chat] runtime rejected run (pre-run failure): tenant=%s session=%s code=%s body=%s",
					tenantID, sessionID, code, httpErr.Body)
				respondErrorCode(c, attachmentHTTPStatus(code), code, runtimeErrorMessage(code))
				return
			}
		}
		h.saveErrorMessage(tenantID, userID, sessionID, "Runtime 连接失败："+err.Error())
		log.Printf("[chat] runtime stream failed: tenant=%s session=%s err=%v", tenantID, sessionID, err)
		respondError(c, http.StatusBadGateway, "Runtime 连接失败，请稍后重试")
		return
	}
	defer rc.Close()

	// 7. SSE headers + flusher
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

	// 8. Pipe byte-for-byte, buffer for aggregation. A heartbeat goroutine
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
