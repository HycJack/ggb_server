package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestToolCallMessageFormat 检查第二轮请求里 assistant/tool 两条消息的字段，
// 严格网关（对 content:""、缺 type:function、缺 name 都会报 invalid tool_call）。
func TestToolCallMessageFormat(t *testing.T) {
	var second []map[string]any
	rounds := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rounds++
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		if rounds == 1 {
			fmt.Fprintf(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"geogebra_run","arguments":"{\"commands\":[\"c: Circle((0,0),3)\"]}"}}]},"finish_reason":"tool_calls"}]}`+"\n\n")
			return
		}
		var req struct {
			Messages []map[string]any `json:"messages"`
		}
		json.Unmarshal(body, &req)
		second = req.Messages
		fmt.Fprintf(w, `data: {"choices":[{"delta":{"content":"done"},"finish_reason":"stop"}]}`+"\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	a := &app{
		s:            settings{UseTools: true, Model: "m", SystemPrompt: "sys"},
		api:          NewOpenAIClient(srv.URL, "k"),
		msgsOf:       map[uint][]*uiMsg{},
		boardPending: map[string]chan boardResult{},
	}
	a.convID = 1
	reply := &uiMsg{id: "a1", role: roleAssistant, open: true}
	a.msgsOf[1] = []*uiMsg{{id: "u1", role: roleUser, text: "画圆", done: true}, reply}
	a.streamChat("画圆", reply)

	if len(second) < 3 {
		t.Fatalf("第二轮消息数 = %d: %+v", len(second), second)
	}
	assistant := second[len(second)-2]
	tool := second[len(second)-1]

	if assistant["role"] != "assistant" {
		t.Errorf("assistant role = %v", assistant["role"])
	}
	if v, ok := assistant["content"]; !ok || v != nil {
		t.Errorf("assistant content 应为 null，实际 %v (存在=%v)", v, ok)
	}
	calls, _ := assistant["tool_calls"].([]any)
	if len(calls) != 1 {
		t.Fatalf("tool_calls = %v", assistant["tool_calls"])
	}
	call := calls[0].(map[string]any)
	if call["type"] != "function" {
		t.Errorf("tool_call.type = %v (应为 function)", call["type"])
	}
	if call["id"] != "call_1" {
		t.Errorf("tool_call.id = %v", call["id"])
	}
	fn, ok := call["function"].(map[string]any)
	if !ok {
		t.Fatalf("tool_call 缺少嵌套的 function 对象: %v", call)
	}
	if fn["name"] != "geogebra_run" {
		t.Errorf("function.name = %v", fn["name"])
	}
	if _, ok := fn["arguments"].(string); !ok {
		t.Errorf("function.arguments 应为字符串: %v", fn["arguments"])
	}

	if tool["role"] != "tool" {
		t.Errorf("tool role = %v", tool["role"])
	}
	if tool["tool_call_id"] != "call_1" {
		t.Errorf("tool.tool_call_id = %v", tool["tool_call_id"])
	}
	if tool["name"] != "geogebra_run" {
		t.Errorf("tool.name = %v", tool["name"])
	}
	if _, ok := tool["content"].(string); !ok {
		t.Errorf("tool.content 应为字符串: %v", tool["content"])
	}
}
