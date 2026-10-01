package gateway

import (
	"encoding/json"
	"net/url"
)

type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
}
type ToolCall struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Arguments     json.RawMessage `json:"arguments"`
	ResponseID    string          `json:"response_id,omitempty"`
	ResponseState string          `json:"response_status,omitempty"`
}

// ProtocolFor respects explicit protocol selection; auto alone may fall back.
func (c *Client) ProtocolFor(model string) string {
	switch c.Mode {
	case "chat":
		return "chat"
	case "anthropic":
		return "messages"
	case "openai":
		return "responses"
	default:
		return Protocol(model)
	}
}
func toolWire(q CallRequest, protocol string) ([]any, any) {
	var defs []any
	for _, t := range q.Tools {
		switch protocol {
		case "messages":
			defs = append(defs, map[string]any{"name": t.Name, "description": t.Description, "input_schema": t.Parameters})
		case "responses":
			defs = append(defs, map[string]any{"type": "function", "name": t.Name, "description": t.Description, "parameters": t.Parameters})
		default:
			defs = append(defs, map[string]any{"type": "function", "function": map[string]any{"name": t.Name, "description": t.Description, "parameters": t.Parameters}})
		}
	}
	if q.WebSearch {
		if protocol == "messages" {
			defs = append(defs, map[string]any{"type": "web_search_20250305", "name": "web_search", "max_uses": 1})
		} else if protocol == "responses" {
			defs = append(defs, map[string]any{"type": "web_search", "search_context_size": "low"})
		}
	}
	var choice any
	if q.ForceTool != "" {
		switch protocol {
		case "messages":
			choice = map[string]string{"type": "tool", "name": q.ForceTool}
		case "responses":
			choice = map[string]string{"type": "function", "name": q.ForceTool}
		default:
			// `required` is accepted by more OpenAI-compatible gateways than
			// the newer forced-function object. With one declared tool it still
			// leaves the model no other tool to choose.
			choice = "required"
		}
	}
	return defs, choice
}
func extractToolEvidence(raw []byte, protocol string, m *Metrics) {
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return
	}
	addURL := func(value any) {
		text, ok := value.(string)
		if !ok {
			return
		}
		u, e := url.Parse(text)
		if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return
		}
		for _, prev := range m.SearchURLs {
			if prev == text {
				return
			}
		}
		m.SearchURLs = append(m.SearchURLs, text)
	}
	if protocol == "messages" {
		content, _ := root["content"].([]any)
		searchIDs := map[string]bool{}
		for _, entry := range content {
			b, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			kind, _ := b["type"].(string)
			name, _ := b["name"].(string)
			id, _ := b["id"].(string)
			if kind == "tool_use" {
				args, _ := json.Marshal(b["input"])
				m.ToolCalls = append(m.ToolCalls, ToolCall{ID: id, Name: name, Arguments: args})
			}
			if kind == "server_tool_use" && name == "web_search" && id != "" {
				searchIDs[id] = true
				m.WebSearchInvoked = true
			}
		}
		for _, entry := range content {
			b, ok := entry.(map[string]any)
			if !ok || b["type"] != "web_search_tool_result" {
				continue
			}
			id, _ := b["tool_use_id"].(string)
			if !searchIDs[id] {
				continue
			}
			results, _ := b["content"].([]any)
			for _, entry := range results {
				hit, ok := entry.(map[string]any)
				if ok && hit["type"] == "web_search_result" {
					addURL(hit["url"])
					m.WebSearchCompleted = true
				}
			}
		}
		return
	}
	if protocol == "responses" {
		output, _ := root["output"].([]any)
		for _, entry := range output {
			b, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			if b["type"] == "function_call" {
				id, _ := b["call_id"].(string)
				name, _ := b["name"].(string)
				responseID, _ := b["id"].(string)
				responseStatus, _ := b["status"].(string)
				args := rawArguments(b["arguments"])
				if json.Valid(args) {
					m.ToolCalls = append(m.ToolCalls, ToolCall{ID: id, Name: name, Arguments: args, ResponseID: responseID, ResponseState: responseStatus})
				}
			}
			if b["type"] == "web_search_call" && b["status"] == "completed" {
				m.WebSearchCompleted = true
			}
			if b["type"] == "message" {
				content, _ := b["content"].([]any)
				for _, part := range content {
					p, ok := part.(map[string]any)
					if !ok || p["type"] != "output_text" {
						continue
					}
					annotations, _ := p["annotations"].([]any)
					for _, item := range annotations {
						a, ok := item.(map[string]any)
						if ok && a["type"] == "url_citation" {
							addURL(a["url"])
						}
					}
				}
			}
		}
		return
	}
	choices, _ := root["choices"].([]any)
	for _, entry := range choices {
		b, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		msg, _ := b["message"].(map[string]any)
		calls, _ := msg["tool_calls"].([]any)
		for _, item := range calls {
			call, ok := item.(map[string]any)
			if !ok || call["type"] != "function" {
				continue
			}
			f, _ := call["function"].(map[string]any)
			name, _ := f["name"].(string)
			args := rawArguments(f["arguments"])
			id, _ := call["id"].(string)
			if json.Valid(args) {
				m.ToolCalls = append(m.ToolCalls, ToolCall{ID: id, Name: name, Arguments: args})
			}
		}
	}
}

func rawArguments(value any) json.RawMessage {
	switch v := value.(type) {
	case string:
		return json.RawMessage(v)
	case nil:
		return nil
	default:
		b, _ := json.Marshal(v)
		return b
	}
}

// PinProtocol keeps a successful diagnostic sequence on one wire format.
func (c *Client) PinProtocol(protocol string) {
	switch protocol {
	case "messages":
		c.Mode = "anthropic"
	case "responses":
		c.Mode = "openai"
	case "chat":
		c.Mode = "chat"
	}
}

// ToolReply sends a safe synthetic result back to the exact received call ID.
type ToolReply struct {
	Call   ToolCall
	Output string
}
