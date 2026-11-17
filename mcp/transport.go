package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// Transport sends JSON-RPC requests and returns responses.
type Transport interface {
	Call(ctx context.Context, method string, params any) (json.RawMessage, error)
	Close() error
}

// StdioTransport runs an MCP server as a child process (Content-Length framing).
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex
	nextID atomic.Int64
}

// NewStdio starts command with args and env extras.
func NewStdio(command string, args []string, env []string) (*StdioTransport, error) {
	cmd := exec.Command(command, args...)
	if len(env) > 0 {
		cmd.Env = append(cmd.Environ(), env...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	cmd.Stderr = nil // discard; hosts may wire later
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, err
	}
	return &StdioTransport{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
	}, nil
}

func (t *StdioTransport) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	id := int(t.nextID.Add(1))
	req := rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	frame := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	if _, err := io.WriteString(t.stdin, frame); err != nil {
		return nil, err
	}

	type result struct {
		raw json.RawMessage
		err error
	}
	ch := make(chan result, 1)
	go func() {
		raw, err := t.readResponse(id)
		ch <- result{raw, err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		return r.raw, r.err
	}
}

func (t *StdioTransport) readResponse(wantID int) (json.RawMessage, error) {
	for {
		var contentLen int
		for {
			line, err := t.stdout.ReadString('\n')
			if err != nil {
				return nil, err
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break
			}
			if strings.HasPrefix(strings.ToLower(line), "content-length:") {
				n := strings.TrimSpace(line[len("Content-Length:"):])
				contentLen, err = strconv.Atoi(strings.TrimSpace(n))
				if err != nil {
					return nil, err
				}
			}
		}
		if contentLen <= 0 {
			return nil, fmt.Errorf("mcp stdio: missing Content-Length")
		}
		buf := make([]byte, contentLen)
		if _, err := io.ReadFull(t.stdout, buf); err != nil {
			return nil, err
		}
		var resp rpcResponse
		if err := json.Unmarshal(buf, &resp); err != nil {
			return nil, err
		}
		// Skip notifications (no id)
		if resp.ID == nil {
			continue
		}
		if *resp.ID != wantID {
			continue
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("mcp rpc: %s", resp.Error.Message)
		}
		return resp.Result, nil
	}
}

func (t *StdioTransport) Notify(_ context.Context, method string, params any) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	req := rpcRequest{JSONRPC: "2.0", Method: method, Params: params}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	frame := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	_, err = io.WriteString(t.stdin, frame)
	return err
}

func (t *StdioTransport) Close() error {
	_ = t.stdin.Close()
	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
		_, _ = t.cmd.Process.Wait()
	}
	return nil
}

// HTTPTransport is a Streamable HTTP MCP client (JSON POST).
type HTTPTransport struct {
	url     string
	headers map[string]string
	client  *http.Client
	nextID  atomic.Int64
	mu      sync.Mutex
	session string
}

// NewHTTP creates an HTTP transport.
func NewHTTP(url string, headers map[string]string) *HTTPTransport {
	return &HTTPTransport{
		url:     url,
		headers: headers,
		client:  &http.Client{},
	}
}

func (t *HTTPTransport) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	id := int(t.nextID.Add(1))
	reqBody := rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range t.headers {
		httpReq.Header.Set(k, v)
	}
	if t.session != "" {
		httpReq.Header.Set("Mcp-Session-Id", t.session)
	}

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		t.session = sid
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("mcp http %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}

	// SSE: take last data JSON line; else plain JSON
	payload := raw
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") {
		payload = extractSSEData(raw)
	}

	var rpcResp rpcResponse
	if err := json.Unmarshal(payload, &rpcResp); err != nil {
		return nil, fmt.Errorf("mcp http decode: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("mcp rpc: %s", rpcResp.Error.Message)
	}
	return rpcResp.Result, nil
}

func extractSSEData(raw []byte) []byte {
	var last []byte
	for _, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimRight(line, "\r")
		if bytes.HasPrefix(line, []byte("data:")) {
			last = bytes.TrimSpace(line[5:])
		}
	}
	if last != nil {
		return last
	}
	return raw
}

func (t *HTTPTransport) Notify(ctx context.Context, method string, params any) error {
	// Best-effort: some HTTP MCP servers accept notifications as POST without id.
	reqBody := rpcRequest{JSONRPC: "2.0", Method: method, Params: params}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range t.headers {
		httpReq.Header.Set(k, v)
	}
	if t.session != "" {
		httpReq.Header.Set("Mcp-Session-Id", t.session)
	}
	resp, err := t.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func (t *HTTPTransport) Close() error {
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
