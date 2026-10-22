package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/HycJack/MujicaUI/chat"
	"github.com/egoist/mygo/ui"
)

// screenshot 渲染一帧状态并写入 PNG，返回 tester（供断言）与文件路径。
func screenshot(t *testing.T, name string, a *app) (*ui.Tester, string) {
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
	return tester, p
}

func demoApp() *app {
	a := newApp()
	a.s.APIBase = "https://api.deepseek.com"
	a.s.APIKey = "sk-demo"
	a.s.Model = "deepseek-chat"
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
			text:     "题目类型：**函数**\n\n提取的数学元素：\n- 二次函数 f(x) = x² − 3x + 1\n- 顶点 V(1.5, −1.25)\n- 与 y 轴交点 (0, 1)\n\n```ggb\nf(x) = x^2 - 3x + 1\nV = Vertex(f)\n```",
			ggb:      "f(x) = x^2 - 3x + 1\nV = Vertex(f)",
		},
	}
}

func TestScreenshotWelcome(t *testing.T) {
	a := demoApp()
	a.convID = 0 // 走欢迎页分支（未选会话）
	a.convSel.Selected = ""
	tester, p := screenshot(t, "welcome", a)
	if !tester.HasText("画一道数学题") {
		t.Errorf("welcome greeting missing in %q", tester.Texts())
	}
	t.Log("written", p)
}

func TestScreenshotSettings(t *testing.T) {
	a := newApp()
	a.s.APIBase, a.s.Model = "", "" // 未配置状态（不依赖磁盘上的真实设置）
	tester, p := screenshot(t, "settings", a)
	if !tester.HasText("先配置一个 OpenAI 兼容接口") {
		t.Errorf("setup hint missing in %q", tester.Texts())
	}
	t.Log("written", p)
}

func TestScreenshotConversation(t *testing.T) {
	a := demoApp()
	a.msgsOf[1] = demoThread()
	// 第一条用户消息带图：用程序生成的位图代替真实文件
	photo := image.NewRGBA(image.Rect(0, 0, 160, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 160; x++ {
			photo.Set(x, y, color.RGBA{uint8(x * 255 / 160), uint8(y * 255 / 120), 120, 255})
		}
	}
	a.msgsOf[1][0].bitmap = ui.NewBitmap(photo)
	a.boardApplied = 2
	tester, p := screenshot(t, "conversation", a)
	if !tester.HasText("GeoGebra 画板") {
		t.Errorf("board panel missing in %q", tester.Texts())
	}
	t.Log("written", p)
}

func TestScreenshotStreaming(t *testing.T) {
	a := demoApp()
	a.msgsOf[1] = append(demoThread(), &uiMsg{
		id: "u2", role: chat.MessageUser, text: "画一个正方体并标出对角线。", done: true,
	}, &uiMsg{
		id: "a2", role: chat.MessageAssistant,
		thinking: "正方体有 8 个顶点、12 条棱。对角线连接不相邻的顶点，例如 A 到 G…",
		text:     "正方体 ABCD-EFGH，棱长 3，连接对角线 AG。",
		open:     true,
		started:  time.Now(),
	})
	_, p := screenshot(t, "streaming", a)
	t.Log("written", p)
}
