package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// 消息角色（对应 uiMsg.role）。
const (
	roleUser int = iota
	roleAssistant
)

// conversation 是侧栏会话条目；消息体在 app.msgsOf 里，本地持久化。
type conversation struct {
	ID        uint
	Title     string
	UpdatedAt time.Time
}

// legacySystemPrompt 是第一版默认提示词；升级时自动替换为新默认。
const legacySystemPrompt = "你是数学画图助手。涉及几何/函数作图时，先把思路讲清楚，" +
	"再把 GeoGebra 指令逐行放进 ```ggb 围栏代码块中（例如：\n```ggb\nf(x) = x^2\n```\n）。"

// settings 是落盘的配置：OpenAI 兼容端点与画板行为。
type settings struct {
	APIBase      string `json:"apiBase"`
	APIKey       string `json:"apiKey"`
	Model        string `json:"model"`
	SystemPrompt string `json:"systemPrompt"`
	AutoBoard    bool   `json:"autoBoard"` // 自动把 ```ggb 围栏画到画板（工具关闭时的兜底）
	UseTools     bool   `json:"useTools"`  // ReAct 工具调用；旧配置文件缺失时回填 true
}

func settingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ggb-desktop", "settings.json"), nil
}

// DefaultSystemPrompt 明确告诉模型：它有一块实时 GeoGebra 画板，
// 把指令写进 ```ggb 围栏应用就会自动执行——没有这句，模型常以
// "我无法操作 GeoGebra" 拒绝作图。
const DefaultSystemPrompt = "你是一个内嵌在桌面应用里的数学画图助手，应用自带一块实时 GeoGebra 画板，你完全可以操作它。优先调用工具 geogebra_run 在画板上执行 GeoGebra 指令（也可以把指令写进 ```ggb 围栏代码块，应用会自动执行）；geogebra_read 可查询画板已有的对象；geogebra_export 仅在用户明确要求保存文件时调用。作图规范：每行一条指令，先创建对象再引用，常用写法如 A=(1,2)、Segment(A,B)、Circle(A,2)、Polygon(A,B,C)、f(x)=sin(x)、Intersect(f,g)；需要立体图形时直接用 3D 指令（Sphere、Cube、Pyramid、Prism 等）并指定 perspective=AT；可用 ShowLabel / SetColor / SetCaption 控制标注与样式。回答结构：先用一两句话讲思路，然后调用工具（或给指令围栏），最后根据工具返回的结果总结画了什么；若指令执行失败，修正后重试。如果用户没给完整条件，按最常见的方式补全并说明。"

func defaultSettings() settings {
	return settings{
		APIBase:      "https://api.deepseek.com",
		Model:        "deepseek-chat",
		AutoBoard:    true,
		UseTools:     true,
		SystemPrompt: DefaultSystemPrompt,
	}
}

func loadSettings() settings {
	s := defaultSettings()
	if p, err := settingsPath(); err == nil {
		if data, err := os.ReadFile(p); err == nil {
			json.Unmarshal(data, &s)
			// 旧版本文件没有 useTools 字段：默认开启 ReAct
			var raw map[string]json.RawMessage
			if json.Unmarshal(data, &raw) == nil {
				if _, ok := raw["useTools"]; !ok {
					s.UseTools = true
				}
			}
			// 旧版本文件没有 systemPrompt 字段，或存的是旧版弱提示词：都升级到新默认
			if s.SystemPrompt == "" || s.SystemPrompt == legacySystemPrompt {
				s.SystemPrompt = DefaultSystemPrompt
			}
		}
	}
	return s
}

func saveSettings(s settings) {
	p, err := settingsPath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(p, data, 0o600)
}

// ---- 本地会话持久化 ----

// storedMsg 是落盘的消息（图片不落盘，重开只保留文字）。
type storedMsg struct {
	Role     int    `json:"role"` // roleUser/roleAssistant
	Text     string `json:"text"`
	Thinking string `json:"thinking,omitempty"`
	GGB      string `json:"ggb,omitempty"`
}

type storedConv struct {
	ID    uint        `json:"id"`
	Title string      `json:"title"`
	Msgs  []storedMsg `json:"msgs"`
}

func historyPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ggb-desktop", "history.json"), nil
}

func loadHistory() (map[uint][]*uiMsg, []conversation, error) {
	msgs := map[uint][]*uiMsg{}
	var convs []conversation
	p, err := historyPath()
	if err != nil {
		return msgs, convs, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return msgs, convs, err
	}
	var stored []storedConv
	if err := json.Unmarshal(data, &stored); err != nil {
		return msgs, convs, err
	}
	for _, sc := range stored {
		conv := storedToUi(sc)
		convs = append(convs, conversation{ID: sc.ID, Title: sc.Title})
		msgs[sc.ID] = conv
	}
	return msgs, convs, nil
}

func storedToUi(sc storedConv) []*uiMsg {
	var out []*uiMsg
	for _, sm := range sc.Msgs {
		out = append(out, &uiMsg{
			id:       "s" + strconv.Itoa(len(out)),
			role:     sm.Role,
			text:     sm.Text,
			thinking: sm.Thinking,
			ggb:      sm.GGB,
			done:     true,
		})
	}
	return out
}

func saveHistory(msgs map[uint][]*uiMsg, convs []conversation) {
	p, err := historyPath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	var stored []storedConv
	for _, cv := range convs {
		sc := storedConv{ID: cv.ID, Title: cv.Title}
		for _, m := range msgs[cv.ID] {
			sm := storedMsg{Role: int(m.role), Text: m.text, Thinking: m.thinking, GGB: m.ggb}
			if sm.Text == "" && sm.Thinking == "" && sm.GGB == "" {
				continue
			}
			sc.Msgs = append(sc.Msgs, sm)
		}
		stored = append(stored, sc)
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(p, data, 0o600)
}

// uiMsg 是视图层的消息：一条用户提问或一条助手回复。
type uiMsg struct {
	id   string
	role int

	text     string // 用户正文 / 助手的 markdown 输出
	thinking string // reasoning 增量累积
	ggb      string // 最近一次抽取到的 GGB 指令（围栏）
	html     string // 最近一次抽取到的完整 HTML（```html 围栏）
	errText  string // 失败信息

	step      string // 进行中的状态标签
	done      bool
	open      bool      // 思考块默认展开
	started   time.Time // 回复开始时间
	usedTools bool      // 本条回复用过画板工具（围栏兜底跳过）

	bitmap  *ui.Bitmap // 用户消息的图片（本地预览）
	dataURL string     // 发给视觉模型的 data URL（不落盘）
}

// stepLabel 保留 streaming 提示的措辞。
func stepLabel(step string) string {
	if step == "" {
		return "思考中"
	}
	return step
}

func truncTitle(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > 24 {
		return string(r[:24]) + "…"
	}
	return string(r)
}

var _ = time.Now
