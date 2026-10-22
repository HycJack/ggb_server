package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestReasoningStream 验证 delta.reasoning_content（DeepSeek 风格）与
// delta.reasoning（OpenRouter 风格）都会被接收。
func TestReasoningStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"先想\"}}]}\n\n")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"reasoning\":\"考一下\"}}]}\n\n")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"答案\"}}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	var reasoning, content string
	client := NewOpenAIClient(srv.URL, "k")
	err := client.Chat("m", []OAMessage{textMessage("user", "hi")}, func(ev OAEvent) {
		reasoning += ev.Reasoning
		content += ev.Content
	})
	if err != nil {
		t.Fatal(err)
	}
	if reasoning != "先想考一下" {
		t.Errorf("reasoning = %q", reasoning)
	}
	if content != "答案" {
		t.Errorf("content = %q", content)
	}
}
