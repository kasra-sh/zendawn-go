package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kasra-sh/zendawn-go/session"
	"github.com/kasra-sh/zendawn-go/tool"
	"github.com/kasra-sh/zendawn-go/zerr"
)

// OpenAI is a minimal OpenAI-compatible chat completions provider.
type OpenAI struct {
	BaseURL    string
	APIKey     string // literal key; preferred when set
	APIKeyEnv  string
	HTTPClient *http.Client
	name       string
}

// NewOpenAI creates a provider. name is the registry id (e.g. "openai").
func NewOpenAI(name, baseURL, apiKeyEnv string) *OpenAI {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if apiKeyEnv == "" {
		apiKeyEnv = "OPENAI_API_KEY"
	}
	return &OpenAI{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKeyEnv:  apiKeyEnv,
		HTTPClient: &http.Client{Timeout: 120 * time.Second},
		name:       name,
	}
}

// WithAPIKey sets a literal API key (overrides env when non-empty).
func (o *OpenAI) WithAPIKey(key string) *OpenAI {
	o.APIKey = key
	return o
}

func (o *OpenAI) resolveAPIKey() string {
	if o.APIKey != "" {
		return o.APIKey
	}
	return os.Getenv(o.APIKeyEnv)
}

func (o *OpenAI) ID() string { return o.name }

func (o *OpenAI) Capabilities() Capabilities {
	return Capabilities{Tools: true, Streaming: true, Images: true}
}

func (o *OpenAI) Complete(ctx context.Context, req Request, onEvent StreamHandler) (Response, error) {
	key := o.resolveAPIKey()
	if key == "" {
		return Response{}, zerr.New(zerr.CodeAuthFailed, "missing API key (apiKey or env "+o.APIKeyEnv+")", false)
	}

	msgs, err := projectMessages(req.Instructions, req.Messages, req.ResolveAttachment)
	if err != nil {
		return Response{}, err
	}
	body := map[string]any{
		"model":    req.Model,
		"messages": msgs,
	}
	if len(req.Tools) > 0 {
		body["tools"] = projectTools(req.Tools)
	}
	for k, v := range req.Params {
		body[k] = v
	}
	if req.Stream {
		body["stream"] = true
		// Ask OpenAI-compatible APIs to include a final usage chunk.
		body["stream_options"] = map[string]any{"include_usage": true}
	}

	jb, err := json.Marshal(body)
	if err != nil {
		return Response{}, zerr.New(zerr.CodeInvalidRequest, err.Error(), false)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.BaseURL+"/chat/completions", bytes.NewReader(jb))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+key)
	httpReq.Header.Set("Content-Type", "application/json")

	res, err := o.HTTPClient.Do(httpReq)
	if err != nil {
		return Response{}, zerr.New(zerr.CodeUnavailable, err.Error(), true)
	}
	defer res.Body.Close()

	if res.StatusCode == 429 {
		return Response{}, zerr.New(zerr.CodeRateLimit, "rate limited", true)
	}
	if res.StatusCode >= 500 {
		return Response{}, zerr.New(zerr.CodeUnavailable, fmt.Sprintf("status %d", res.StatusCode), true)
	}
	if res.StatusCode >= 400 {
		b, _ := io.ReadAll(res.Body)
		return Response{}, zerr.New(zerr.CodeInvalidRequest, string(b), false)
	}

	if req.Stream {
		return o.readStream(res.Body, onEvent)
	}

	rawBody, err := io.ReadAll(res.Body)
	if err != nil {
		return Response{}, zerr.New(zerr.CodeInternal, err.Error(), false)
	}
	var raw openAIResponse
	if err := json.Unmarshal(rawBody, &raw); err != nil {
		return Response{}, zerr.New(zerr.CodeInternal, err.Error(), false)
	}
	var top map[string]any
	_ = json.Unmarshal(rawBody, &top)
	usage := ParseUsage(raw.Usage, top)
	msg := raw.toItem()
	if onEvent != nil {
		onEvent("provider.message_completed", map[string]any{"message": msg})
	}
	return Response{
		ID:           raw.ID,
		Message:      msg,
		FinishReason: raw.finishReason(),
		Usage:        usage,
	}, nil
}

func (o *OpenAI) readStream(r io.Reader, onEvent StreamHandler) (Response, error) {
	sc := bufio.NewScanner(r)
	// Usage chunks can be large; raise the buffer.
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	var text strings.Builder
	toolCalls := map[int]*session.ContentPart{}
	var finish string
	var id string
	var usage Usage

	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			break
		}
		var top map[string]any
		_ = json.Unmarshal([]byte(payload), &top)
		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.ID != "" {
			id = chunk.ID
		}
		if len(chunk.Usage) > 0 && string(chunk.Usage) != "null" {
			usage = ParseUsage(chunk.Usage, top)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if delta.Content != "" {
			text.WriteString(delta.Content)
			if onEvent != nil {
				onEvent("provider.text_delta", map[string]any{"text": delta.Content})
			}
		}
		for _, tc := range delta.ToolCalls {
			idx := tc.Index
			p := toolCalls[idx]
			if p == nil {
				p = &session.ContentPart{Type: session.PartToolCall}
				toolCalls[idx] = p
			}
			if tc.ID != "" {
				p.ToolCallID = tc.ID
			}
			if tc.Function.Name != "" {
				p.Name = tc.Function.Name
			}
			p.Arguments = concatArgs(p.Arguments, tc.Function.Arguments)
		}
		if chunk.Choices[0].FinishReason != "" {
			finish = chunk.Choices[0].FinishReason
		}
	}

	parts := []session.ContentPart{}
	if text.Len() > 0 {
		parts = append(parts, session.ContentPart{Type: session.PartText, Text: text.String()})
	}
	for i := 0; i < len(toolCalls); i++ {
		if p, ok := toolCalls[i]; ok {
			p.Arguments = parseArgs(p.Arguments)
			parts = append(parts, *p)
			if onEvent != nil {
				onEvent("provider.tool_call_completed", map[string]any{
					"toolCallId": p.ToolCallID, "name": p.Name, "arguments": p.Arguments,
				})
			}
		}
	}
	msg := session.Item{Type: session.ItemMessage, Role: session.RoleAssistant, Content: parts, CreatedAt: time.Now().UTC()}
	if onEvent != nil {
		onEvent("provider.message_completed", map[string]any{"message": msg})
	}
	if finish == "tool_calls" || len(session.ToolCalls(parts)) > 0 {
		finish = "tool_calls"
	} else if finish == "" {
		finish = "stop"
	}
	return Response{ID: id, Message: msg, FinishReason: finish, Usage: usage}, nil
}

func concatArgs(existing any, add string) any {
	prev, _ := existing.(string)
	return prev + add
}

func parseArgs(v any) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return s
	}
	return m
}

func projectMessages(instructions string, items []session.Item, resolve AttachmentResolver) ([]map[string]any, error) {
	out := []map[string]any{}
	if instructions != "" {
		out = append(out, map[string]any{"role": "system", "content": instructions})
	}
	for _, it := range items {
		switch it.Type {
		case session.ItemMessage:
			if it.Role == session.RoleAssistant {
				content := ""
				var tcs []map[string]any
				for _, p := range it.Content {
					switch p.Type {
					case session.PartText:
						content += p.Text
					case session.PartToolCall:
						args, _ := json.Marshal(p.Arguments)
						tcs = append(tcs, map[string]any{
							"id":   p.ToolCallID,
							"type": "function",
							"function": map[string]any{
								"name":      p.Name,
								"arguments": string(args),
							},
						})
					}
				}
				m := map[string]any{"role": "assistant"}
				if content != "" {
					m["content"] = content
				} else {
					m["content"] = nil
				}
				if len(tcs) > 0 {
					m["tool_calls"] = tcs
				}
				out = append(out, m)
			} else {
				content, err := projectUserContent(it.Content, resolve)
				if err != nil {
					return nil, err
				}
				out = append(out, map[string]any{
					"role":    it.Role,
					"content": content,
				})
			}
		case session.ItemToolResult:
			out = append(out, map[string]any{
				"role":         "tool",
				"tool_call_id": it.ToolCallID,
				"content":      session.TextParts(it.Content),
			})
		case session.ItemHandoff:
			out = append(out, map[string]any{
				"role":    "system",
				"content": fmt.Sprintf("Handed off from %s to %s: %s", it.From, it.To, it.Reason),
			})
		}
	}
	return out, nil
}

func projectUserContent(parts []session.ContentPart, resolve AttachmentResolver) (any, error) {
	hasRich := false
	for _, p := range parts {
		if p.Type == session.PartImage || p.Type == session.PartFile {
			hasRich = true
			break
		}
	}
	if !hasRich {
		return session.TextParts(parts), nil
	}
	var out []map[string]any
	for _, p := range parts {
		switch p.Type {
		case session.PartText:
			if p.Text != "" {
				out = append(out, map[string]any{"type": "text", "text": p.Text})
			}
		case session.PartFile:
			stub, err := fileStub(p, resolve)
			if err != nil {
				return nil, err
			}
			out = append(out, map[string]any{"type": "text", "text": stub})
		case session.PartImage:
			block, err := imageURLBlock(p, resolve)
			if err != nil {
				return nil, err
			}
			out = append(out, block)
		}
	}
	if len(out) == 0 {
		return "", nil
	}
	return out, nil
}

func fileStub(p session.ContentPart, resolve AttachmentResolver) (string, error) {
	filename, mimeType, relPath := p.Filename, p.MimeType, ""
	if p.AttachmentID != "" && resolve != nil {
		fn, mime, rel, _, err := resolve(p.AttachmentID)
		if err != nil {
			return "", zerr.New(zerr.CodeInvalidRequest, err.Error(), false)
		}
		if filename == "" {
			filename = fn
		}
		if mimeType == "" {
			mimeType = mime
		}
		relPath = rel
	}
	if relPath == "" && p.AttachmentID != "" && filename != "" {
		relPath = fmt.Sprintf(".zendawn/uploads/%s/%s", p.AttachmentID, filename)
	}
	if relPath == "" {
		relPath = filename
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return fmt.Sprintf("Attached file: %s (%s)", relPath, mimeType), nil
}

func imageURLBlock(p session.ContentPart, resolve AttachmentResolver) (map[string]any, error) {
	mimeType := p.MimeType
	url := ""
	if p.URI != "" {
		url = p.URI
	} else if p.Data != "" {
		if mimeType == "" {
			mimeType = "image/png"
		}
		url = "data:" + mimeType + ";base64," + p.Data
	} else if p.AttachmentID != "" && resolve != nil {
		_, mime, _, data, err := resolve(p.AttachmentID)
		if err != nil {
			return nil, zerr.New(zerr.CodeInvalidRequest, err.Error(), false)
		}
		if mimeType == "" {
			mimeType = mime
		}
		if mimeType == "" {
			mimeType = "image/png"
		}
		url = "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
	} else {
		return nil, zerr.New(zerr.CodeInvalidRequest, "image part missing data, uri, or attachmentId", false)
	}
	return map[string]any{
		"type": "image_url",
		"image_url": map[string]any{
			"url": url,
		},
	}, nil
}

func projectTools(defs []tool.Definition) []map[string]any {
	out := make([]map[string]any, 0, len(defs))
	for _, d := range defs {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        d.Name,
				"description": d.Description,
				"parameters":  d.InputSchema,
			},
		})
	}
	return out
}

type openAIResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage json.RawMessage `json:"usage"`
}

func (r openAIResponse) finishReason() string {
	if len(r.Choices) == 0 {
		return "stop"
	}
	fr := r.Choices[0].FinishReason
	if fr == "tool_calls" {
		return "tool_calls"
	}
	return "stop"
}

func (r openAIResponse) toItem() session.Item {
	parts := []session.ContentPart{}
	if len(r.Choices) == 0 {
		return session.Item{Type: session.ItemMessage, Role: session.RoleAssistant, Content: parts, CreatedAt: time.Now().UTC()}
	}
	m := r.Choices[0].Message
	if m.Content != "" {
		parts = append(parts, session.ContentPart{Type: session.PartText, Text: m.Content})
	}
	for _, tc := range m.ToolCalls {
		parts = append(parts, session.ContentPart{
			Type:       session.PartToolCall,
			ToolCallID: tc.ID,
			Name:       tc.Function.Name,
			Arguments:  parseArgs(tc.Function.Arguments),
		})
	}
	return session.Item{Type: session.ItemMessage, Role: session.RoleAssistant, Content: parts, CreatedAt: time.Now().UTC()}
}

type openAIStreamChunk struct {
	ID      string `json:"id"`
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage json.RawMessage `json:"usage"`
}
