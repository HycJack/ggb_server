package main

import (
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/HycJack/MujicaUI/chat"
	"github.com/egoist/mygo/ui"
)

// screenshot 渲染一帧状态并写入 PNG，返回文件路径。
func screenshot(t *testing.T, name string, a *app) string {
	t.Helper()
	tester := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1180, 740)
	tester.SetScale(1)
	tester.Frame()
	img := tester.Image()
	p := "testdata/" + name + ".png"
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return p
}

func demoApp() *app {
	a := newApp()
	a.logged = true
	a.s.Token = "test-token"
	a.s.Username = "老师甲"
	a.convs = []conversation{
		{ID: 1, Title: "二次函数图像", UpdatedAt: time.Now().Add(-2 * time.Hour)},
		{ID: 2, Title: "正方体的截面问题", UpdatedAt: time.Now().AddDate(0, 0, -1)},
		{ID: 3, Title: "三角函数化简", UpdatedAt: time.Now().AddDate(0, 0, -3)},
	}
	a.convSel.Selected = "1"
	a.convID = 1
	return a
}

func demoThread() []*uiMsg {
	return []*uiMsg{
		{id: "u1", role: chat.MessageUser, text: "画出函数 y = x² − 3x + 1 的图像，并标出顶点。", done: true},
		{
			id: "a1", role: chat.MessageAssistant, done: true, open: true,
			thinking: "这是一道二次函数题。先识别顶点式：y = (x − 1.5)² − 1.25，顶点为 (1.5, −1.25)。需要生成抛物线指令并标注顶点。",
			text:     "题目类型：**函数**\n\n提取的数学元素：\n- 二次函数 f(x) = x² − 3x + 1\n- 顶点 V(1.5, −1.25)\n- 与 y 轴交点 (0, 1)",
			ggb:      "f(x) = x^2 - 3x + 1\nV = Vertex(f)\nSetVisible(f, true)",
			html:     "<!DOCTYPE html><html><body>GeoGebra applet</body></html>",
		},
	}
}

func TestScreenshotLogin(t *testing.T) {
	a := newApp()
	p := screenshot(t, "login", a)
	t.Log("written", p)
}

func TestScreenshotWelcome(t *testing.T) {
	a := demoApp()
	p := screenshot(t, "welcome", a)
	t.Log("written", p)
}

func TestScreenshotConversation(t *testing.T) {
	a := demoApp()
	a.msgs = demoThread()
	a.boardApplied = 3 // 画板占位里的“最近一批指令”
	p := screenshot(t, "conversation", a)
	t.Log("written", p)
}

func TestScreenshotStreaming(t *testing.T) {
	a := demoApp()
	a.msgs = append(demoThread(), &uiMsg{
		id: "u2", role: chat.MessageUser, text: "画一个正方体并标出对角线。", done: true,
	}, &uiMsg{
		id: "a2", role: chat.MessageAssistant,
		step:     "提取数学元素",
		thinking: "正方体有 8 个顶点、12 条棱。对角线连接不相邻的顶点，例如 A 到 G…",
		text:     "正方体 ABCD-EFGH，棱长 3，连接对角线 AG。",
		open:     true,
		started:  time.Now(),
	})
	p := screenshot(t, "streaming", a)
	t.Log("written", p)
}
