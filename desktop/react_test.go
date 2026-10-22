package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestReActLoop 模型第一轮请求工具，第二轮基于工具结果给最终答案：
// 验证循环执行了工具、把结果以 role=tool 回传、并把最终文本落到回复上。
func TestReActLoop(t *testing.T) {
	var sawToolResult bool
	rounds := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rounds++
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
		}
		json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "text/event-stream")
		if rounds == 1 {
			fmt.Fprintf(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"geogebra_run","arguments":"{\"commands\":[\"c: Circle((0,0),3)\"]}"}}]},"finish_reason":"tool_calls"}]}`+"\n\n")
		} else {
			for _, m := range req.Messages {
				if m.Role == "tool" {
					sawToolResult = true
				}
			}
			if !sawToolResult {
				t.Errorf("第二轮请求里没有 role=tool 的结果")
			}
			fmt.Fprintf(w, `data: {"choices":[{"delta":{"content":"圆已画好"},"finish_reason":"stop"}]}`+"\n\n")
		}
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
	a.msgsOf[1] = []*uiMsg{{id: "u1", role: roleUser, text: "画半径 3 的圆", done: true}, reply}

	a.streamChat("画半径 3 的圆", reply)

	if !reply.usedTools {
		t.Errorf("回复未标记 usedTools")
	}
	if reply.text != "圆已画好" {
		t.Errorf("reply.text = %q", reply.text)
	}
	if !reply.done || rounds != 2 {
		t.Errorf("rounds=%d done=%v", rounds, reply.done)
	}
	// headless boardCall 的占位结果：applied=1
	if reply.ggb != "" {
		t.Errorf("工具模式下不应再走围栏兜底")
	}
}
