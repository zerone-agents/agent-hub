package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"
	"unicode/utf8"

	runtimeclient "control-panel/internal/infrastructure/runtime"
)

const (
	proxyContextBodyLimit = 1 << 20
	proxyLegacyBodyLimit  = 16 << 20
	proxyBodyReadTimeout  = 15 * time.Second
)

var (
	errProxyContextInvalid = errors.New("invalid temporary context")
	errProxyContextLarge   = errors.New("temporary context request exceeds limit")
	errProxyBodyTimeout    = errors.New("runtime request body read timed out")
)

// topLevelContextKeys recognizes JSON object keys without retaining message
// string values. It also recognizes escaped key spellings. General malformed
// JSON remains the runtime's responsibility unless it uses a context field.
type topLevelContextKeys struct {
	depth                                        int
	inString, escaped, keyString, wantKey, found bool
	key                                          []byte
}

func (s *topLevelContextKeys) write(data []byte) {
	for _, b := range data {
		if s.inString {
			if s.keyString && len(s.key) < 256 {
				s.key = append(s.key, b)
			}
			if s.escaped {
				s.escaped = false
				continue
			}
			if b == '\\' {
				s.escaped = true
				continue
			}
			if b == '"' {
				s.inString = false
				if s.keyString {
					var key string
					if json.Unmarshal(s.key, &key) == nil && (key == "turnContext" || key == "appendSystemPrompt") {
						s.found = true
					}
				}
			}
			continue
		}
		switch b {
		case '{', '[':
			s.depth++
			if s.depth == 1 && b == '{' {
				s.wantKey = true
			}
		case '}', ']':
			s.depth--
		case ',':
			if s.depth == 1 {
				s.wantKey = true
			}
		case ':':
			if s.depth == 1 {
				s.wantKey = false
			}
		case '"':
			s.inString, s.escaped = true, false
			s.keyString = s.depth == 1 && s.wantKey
			if s.keyString {
				s.key = []byte{'"'}
			}
		}
	}
}

// inspectProxyContextWithDeadline bounds ingress time independently of the
// unlimited generation/SSE deadline. Gin's response writer unwraps to the
// net/http writer; only in-memory test writers may lack deadline support.
func inspectProxyContextWithDeadline(req *http.Request, w http.ResponseWriter, timeout time.Duration) (bool, func(), error) {
	controller := http.NewResponseController(w)
	if err := controller.SetReadDeadline(time.Now().Add(timeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return false, func() {}, errProxyContextInvalid
	}
	defer controller.SetReadDeadline(time.Time{})
	return inspectProxyContext(req)
}

func proxyBodyReadError(err error) error {
	var timed interface{ Timeout() bool }
	if errors.As(err, &timed) && timed.Timeout() {
		return errProxyBodyTimeout
	}
	return errProxyContextInvalid
}

// Run ingress is capped at 16 MiB, including unknown-length/chunked requests;
// context-bearing runs have a stricter 1 MiB limit. Larger ordinary runs use
// an immediately unlinked 0600 spill file (at most 16 MiB), closed on every
// exit. Scanning cannot stop at the first MiB: context may follow a message.
func inspectProxyContext(req *http.Request) (bool, func(), error) {
	if req.Body == nil {
		return false, func() {}, nil
	}
	original := req.Body
	defer original.Close()
	if req.ContentLength > proxyLegacyBodyLimit {
		return false, func() {}, errProxyContextLarge
	}
	// The extra byte distinguishes an exactly-full body from overflow without
	// draining an attacker-controlled stream or growing the spill beyond cap.
	bounded := io.LimitReader(original, proxyLegacyBodyLimit+1)
	prefix, err := io.ReadAll(io.LimitReader(bounded, proxyContextBodyLimit+1))
	if err != nil {
		return false, func() {}, proxyBodyReadError(err)
	}
	scan := topLevelContextKeys{}
	scan.write(prefix)
	if len(prefix) <= proxyContextBodyLimit {
		req.Body = io.NopCloser(bytes.NewReader(prefix))
		req.ContentLength = int64(len(prefix))
		if !scan.found {
			return false, func() {}, nil
		}
		if !utf8.Valid(prefix) {
			return false, func() {}, errProxyContextInvalid
		}
		var body map[string]json.RawMessage
		if json.Unmarshal(prefix, &body) != nil {
			return false, func() {}, errProxyContextInvalid
		}
		a, hasA := body["turnContext"]
		b, hasB := body["appendSystemPrompt"]
		if hasA && hasB {
			return false, func() {}, errProxyContextInvalid
		}
		var context string
		raw := a
		if hasB {
			raw = b
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &context) != nil || utf8.RuneCountInString(context) > runtimeclient.MaxTurnContextRunes {
			return false, func() {}, errProxyContextInvalid
		}
		return true, func() {}, nil
	}
	if scan.found {
		return false, func() {}, errProxyContextLarge
	}
	file, err := os.CreateTemp("", "hub-runtime-body-*")
	if err != nil {
		return false, func() {}, errProxyContextInvalid
	}
	cleanup := func() { file.Close(); os.Remove(file.Name()) }
	// Unlink before writing any request bytes; the descriptor alone owns it.
	if err := os.Remove(file.Name()); err != nil {
		cleanup()
		return false, func() {}, errProxyContextInvalid
	}
	if _, err = file.Write(prefix); err != nil {
		cleanup()
		return false, func() {}, errProxyContextInvalid
	}
	total := int64(len(prefix))
	buf := make([]byte, 32*1024)
	for {
		if req.Context().Err() != nil {
			cleanup()
			return false, func() {}, errProxyContextInvalid
		}
		n, readErr := bounded.Read(buf)
		if n > 0 {
			if total+int64(n) > proxyLegacyBodyLimit {
				cleanup()
				return false, func() {}, errProxyContextLarge
			}
			scan.write(buf[:n])
			if scan.found {
				cleanup()
				return false, func() {}, errProxyContextLarge
			}
			if _, err := file.Write(buf[:n]); err != nil {
				cleanup()
				return false, func() {}, errProxyContextInvalid
			}
			total += int64(n)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			cleanup()
			return false, func() {}, proxyBodyReadError(readErr)
		}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return false, func() {}, errProxyContextInvalid
	}
	req.Body, req.ContentLength = file, total
	return false, cleanup, nil
}
