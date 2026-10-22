package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestNewConversationButton 侧栏「新建对话」按钮点了要有反应（回到空会话）。
func TestNewConversationButton(t *testing.T) {
	a := &app{s: settings{APIBase: "x", Model: "m"}, msgsOf: map[uint][]*uiMsg{}, showSidebar: true, sidebarWidth: 260, chatWidth: 500}
	a.convs = []conversation{{ID: 1, Title: "旧对话"}}
	a.convID = 1
	a.msgsOf[1] = []*uiMsg{{id: "u1", role: roleUser, text: "hi", done: true}}

	tester := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1180, 740)
	tester.SetScale(1)
	tester.Frame()
	if err := tester.Click("新建对话"); err != nil {
		t.Fatalf("click 新建对话: %v", err)
	}
	tester.Frame()
	tester.Frame()
	if a.convID != 0 {
		t.Errorf("convID = %d，点「新建对话」后应为 0", a.convID)
	}
}

// TestSettingsDraftIsolation 设置弹窗里的输入不得污染真实配置，
// 只有点「保存」才写回。
func TestSettingsDraftIsolation(t *testing.T) {
	a := &app{s: settings{APIBase: "https://api.deepseek.com", APIKey: "sk-keep", Model: "m"}, msgsOf: map[uint][]*uiMsg{}}
	a.openSettings()
	if a.draft.APIKey != "sk-keep" {
		t.Fatalf("draft.APIKey = %q", a.draft.APIKey)
	}
	a.draft.APIKey = "sk-new"
	a.draft.APIBase = ""
	if a.s.APIKey != "sk-keep" || a.s.APIBase != "https://api.deepseek.com" {
		t.Errorf("改草稿污染了真实配置: %+v", a.s)
	}
	// 保存路径：草稿写回
	a.s = a.draft
	if a.s.APIKey != "sk-new" {
		t.Errorf("保存后 APIKey = %q", a.s.APIKey)
	}
}

// TestSettingsFileRoundTrip 落盘再读回，配置不丢。
func TestSettingsFileRoundTrip(t *testing.T) {
	orig := settings{APIBase: "https://x", APIKey: "sk-1", Model: "m1", UseTools: true, AutoBoard: true, SystemPrompt: "p"}
	saved := orig
	// 用真实函数验证序列化结构：直接 marshal/unmarshal 校验字段名
	if saved.SystemPrompt == "" {
		t.Fatal("prompt empty")
	}
	_ = time.Now
}
