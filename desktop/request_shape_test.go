package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestChatRequestShape 捕获真实请求体，验证 system 提示词与图片的位置。
func TestChatRequestShape(t *testing.T) {
	var got struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &got)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	a := &app{s: settings{SystemPrompt: "你是数学画图助手", Model: "test-model"}, api: NewOpenAIClient(srv.URL, "sk-test")}
	a.convID = 1
	a.msgsOf = map[uint][]*uiMsg{1: {
		{id: "u1", role: roleUser, text: "第一题", done: true},
		{id: "a1", role: roleAssistant, text: "答案", done: true},
		{id: "u2", role: roleUser, text: "画个圆", dataURL: "data:image/png;base64,AAAA", done: true},
		{id: "a2", role: roleAssistant, done: false},
	}}
	msgs := a.buildMessages("画个圆", a.replyDataImage(a.msgsOf[1][3]))
	if err := a.api.Chat("test-model", msgs, func(OAEvent) {}); err != nil {
		t.Fatal(err)
	}

	if got.Model != "test-model" {
		t.Errorf("model = %q", got.Model)
	}
	if len(got.Messages) == 0 || got.Messages[0].Role != "system" {
		t.Fatalf("第一条应为 system，实际 %+v", got.Messages)
	}
	if string(got.Messages[0].Content) != `"你是数学画图助手"` {
		t.Errorf("system content = %s", got.Messages[0].Content)
	}
	last := got.Messages[len(got.Messages)-1]
	var parts []map[string]any
	if err := json.Unmarshal(last.Content, &parts); err != nil {
		t.Fatalf("带图消息 content 应为数组: %v (%s)", err, last.Content)
	}
	if len(parts) != 2 || parts[0]["type"] != "text" || parts[1]["type"] != "image_url" {
		t.Errorf("content parts = %v", parts)
	}
	t.Logf("请求共 %d 条消息，最后一条为 text+image_url ✓", len(got.Messages))
}

// TestFirstMessageNoDuplicate 首条消息（会话只有刚追加的 user+placeholder）不应把用户文本发两遍。
func TestFirstMessageNoDuplicate(t *testing.T) {
	a := &app{s: settings{SystemPrompt: "sys"}}
	a.convID = 1
	a.msgsOf = map[uint][]*uiMsg{1: {
		{id: "u1", role: roleUser, text: "画个圆", done: true},
		{id: "a1", role: roleAssistant, done: false},
	}}
	msgs := a.buildMessages("画个圆", "")
	userCount := 0
	for _, m := range msgs {
		if m.Role == "user" {
			userCount++
		}
	}
	if userCount != 1 {
		t.Errorf("user 消息出现 %d 次（应为 1）：%+v", userCount, msgs)
	}
}
