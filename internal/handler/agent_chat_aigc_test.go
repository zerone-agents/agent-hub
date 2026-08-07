package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const aigcSSEWithBoth = `event: system
data: {"type":"system","subtype":"init","session_id":"s1","aigc":{"Label":"1","ContentProducer":"001191320118MAK93FC72D10001","ProduceID":"p-init"}}

event: partial_message
data: {"type":"partial_message"}

event: result
data: {"type":"result","aigc":{"Label":"1","ContentProducer":"001191320118MAK93FC72D10001","ProduceID":"p-final"},"aigcExplicitHint":true}

`

const aigcSSESystemOnly = `event: system
data: {"type":"system","subtype":"init","session_id":"s1","aigc":{"Label":"1","ProduceID":"p-init"}}

event: assistant
data: {"message":{"content":[]}}

`

func TestExtractAigcLabel_PrefersResultEvent(t *testing.T) {
	got := extractAigcLabel(aigcSSEWithBoth)
	require.Contains(t, got, "p-final")
	require.NotContains(t, got, "p-init")
}

func TestExtractAigcLabel_FallsBackToSystemInit(t *testing.T) {
	got := extractAigcLabel(aigcSSESystemOnly)
	require.Contains(t, got, "p-init")
}

func TestExtractAigcLabel_NoLabelReturnsEmpty(t *testing.T) {
	got := extractAigcLabel("event: assistant\ndata: {\"message\":{}}\n\n")
	require.Equal(t, "", got)
}

func TestExtractAigcLabel_SkipsMalformedAndNull(t *testing.T) {
	sse := "event: result\ndata: {not json}\n\nevent: result\ndata: {\"type\":\"result\",\"aigc\":null}\n\n"
	require.Equal(t, "", extractAigcLabel(sse))
}
