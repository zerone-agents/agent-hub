package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const runtimeErrorSSE = `event: system
data: {"type":"system","subtype":"init","session_id":"s1"}

event: result
data: {"type":"result","subtype":"error","error_type":"rate_limit","usage":{"input_tokens":0,"output_tokens":0},"num_turns":1,"cost":0,"errors":["429 Your token-plan 1-week quota has been exhausted. The quota will reset at 08-17 01:36:00 UTC."]}

`

func TestExtractRuntimeError_JoinsErrorsFromResultEvent(t *testing.T) {
	got := extractRuntimeError(runtimeErrorSSE)
	require.Contains(t, got, "429")
	require.Contains(t, got, "quota has been exhausted")
}

func TestExtractRuntimeError_JoinsMultipleErrors(t *testing.T) {
	sse := "event: result\ndata: {\"type\":\"result\",\"subtype\":\"error\",\"errors\":[\"first\",\"second\"]}\n\n"
	require.Equal(t, "first\nsecond", extractRuntimeError(sse))
}

func TestExtractRuntimeError_FallsBackToErrorType(t *testing.T) {
	sse := "event: result\ndata: {\"type\":\"result\",\"subtype\":\"error\",\"error_type\":\"rate_limit\"}\n\n"
	require.Equal(t, "rate_limit", extractRuntimeError(sse))
}

func TestExtractRuntimeError_SuccessResultReturnsEmpty(t *testing.T) {
	sse := "event: result\ndata: {\"type\":\"result\",\"subtype\":\"success\",\"usage\":{\"input_tokens\":10}}\n\n"
	require.Equal(t, "", extractRuntimeError(sse))
}

func TestExtractRuntimeError_NoResultEventReturnsEmpty(t *testing.T) {
	sse := "event: assistant\ndata: {\"message\":{\"content\":[]}}\n\n"
	require.Equal(t, "", extractRuntimeError(sse))
}

func TestExtractRuntimeError_SkipsMalformedPayload(t *testing.T) {
	sse := "event: result\ndata: {not json}\n\nevent: result\ndata: {\"type\":\"result\",\"subtype\":\"error\",\"errors\":[\"boom\"]}\n\n"
	require.Equal(t, "boom", extractRuntimeError(sse))
}
