package gateway

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNestedFakeToolDoesNotCount(t *testing.T) {
	var m Metrics
	extractToolEvidence([]byte(`{"content":[{"type":"text","text":"ok","metadata":{"type":"tool_use","id":"fake","name":"audit_echo","input":{"sum":42}}}]}`), "messages", &m)
	if len(m.ToolCalls) != 0 {
		t.Fatal("nested fake tool counted")
	}
}
func TestOrphanWebResultDoesNotCount(t *testing.T) {
	var m Metrics
	extractToolEvidence([]byte(`{"content":[{"type":"server_tool_use","id":"one","name":"web_search"},{"type":"web_search_tool_result","tool_use_id":"other","content":[{"type":"web_search_result","url":"https://example.invalid"}]}]}`), "messages", &m)
	if m.WebSearchCompleted || len(m.SearchURLs) > 0 {
		t.Fatal("orphan search accepted")
	}
}
func TestStreamCacheUsageReplacesNotAdds(t *testing.T) {
	body := "data: {\"type\":\"message_start\",\"message\":{\"content\":[],\"usage\":{\"input_tokens\":10,\"cache_creation_input_tokens\":900,\"cache_read_input_tokens\":0}}}\n\ndata: {\"type\":\"message_delta\",\"usage\":{\"input_tokens\":10,\"cache_creation\":{\"ephemeral_5m_input_tokens\":0,\"ephemeral_1h_input_tokens\":900},\"cache_creation_input_tokens\":900,\"cache_read_input_tokens\":null}}\n\ndata: {\"type\":\"message_stop\"}\n\n"
	for i := 0; i < 30; i++ {
		var m Metrics
		if e := parseStream(strings.NewReader(body), &m); e != nil || m.CacheCreation1h != 900 || m.CacheCreation5m != 0 || m.InputTokens != 10 {
			t.Fatal(m, e)
		}
	}
}
func TestStreamToolCallAssembledFromInputDeltas(t *testing.T) {
	body := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"content\":[],\"usage\":{\"input_tokens\":10}}}\n\n" +
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"audit_echo\",\"input\":{}}}\n\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"nonce\\\":\\\"n\\\",\"}}\n\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"\\\"sum\\\":42}\"}}\n\n" +
		"data: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
		"data: {\"type\":\"message_stop\"}\n\n"
	var m Metrics
	if err := parseStream(strings.NewReader(body), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.ToolCalls) != 1 || m.ToolCalls[0].ID != "toolu_1" || m.ToolCalls[0].Name != "audit_echo" || string(m.ToolCalls[0].Arguments) != `{"nonce":"n","sum":42}` {
		t.Fatalf("unexpected tool calls: %+v", m.ToolCalls)
	}
}
func TestNullUsageNotValidInput(t *testing.T) {
	var m Metrics
	if e := parseMessagesBody([]byte(`{"content":[],"usage":{"input_tokens":null,"cache_read_input_tokens":null,"cache_creation_input_tokens":null}}`), &m); e != nil {
		t.Fatal(e)
	}
	if m.UsagePresent || m.CacheReadReported || m.CacheWriteReported {
		t.Fatal("null counted as reported")
	}
	_ = json.Valid
}
