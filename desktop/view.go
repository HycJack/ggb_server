package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/HycJack/MujicaUI/account"
	"github.com/HycJack/MujicaUI/agent"
	"github.com/HycJack/MujicaUI/chat"
	"github.com/HycJack/MujicaUI/core"
	"github.com/HycJack/MujicaUI/icons"
	"github.com/HycJack/MujicaUI/theme"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

func (a *app) view(c *ui.Context) {
	core.Use(c, core.Settings{Mode: core.System, Locale: core.ZhCN})
	if !a.logged {
		a.loginView(c)
		return
	}
	a.chatView(c)
}

// ---- 登录 ----

func (a *app) loginView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		ui.Column(c).Width(360).Gap(16).Padding(32, 28).Radius(12).Background(t.Surface).
			BorderWidth(1, 1, 1, 1).BorderColor(t.Border).Children(func() {
			ui.Text(c, "GGB 数学画图助手").Font(theme.SerifFont).FontSize(22).Bold().AlignSelf(ui.Center)
			ui.Text(c, "登录后开始把题目变成 GeoGebra 图形").TextColor(t.TextMuted).AlignSelf(ui.Center)

			ui.Column(c).Gap(6).Children(func() {
				ui.Text(c, "服务器地址").FontSize(12).TextColor(t.TextMuted)
				ui.TextInput(c, &a.loginURL).Placeholder("http://localhost:8080")
			})

			account.LoginForm(c, &a.login, account.LoginFormOptions{
				OnSubmit: func(user, pass string, remember bool) {
					a.setClient(a.loginURL)
					a.doLogin(user, pass, remember)
				},
			})
			if a.loginErr != "" {
				ui.Text(c, a.loginErr).TextColor(t.Danger).FontSize(12)
			}
		})
	})
}

// ---- 主界面 ----

func (a *app) chatView(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		// 侧栏：新建对话 + 会话列表 + 用户
		ui.Column(c).Width(272).FillHeight().Padding(12).Gap(10).Background(t.Surface).
			BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Shrink(0).Children(func() {
			ui.Row(c).Gap(8).Children(func() {
				if ui.PrimaryButton(c, "新建对话").Clicked() {
					a.newConversation()
				}
				toggle := ui.Button(c, "画板")
				if a.showBoard {
					toggle = toggle.Label("收起画板")
				}
				if toggle.Clicked() {
					a.showBoard = !a.showBoard
				}
			})

			items := make([]chat.ChatConversation, len(a.convs))
			for i, cv := range a.convs {
				items[i] = chat.ChatConversation{ID: strconv.FormatUint(uint64(cv.ID), 10), Title: cv.Title, Updated: cv.UpdatedAt}
			}
			lv := chat.ConversationList(c, &a.convSel, items, chat.ConversationListOptions{Label: "会话列表"}, nil)
			if lv.Changed() {
				if id, err := strconv.ParseUint(a.convSel.Selected, 10, 64); err == nil {
					a.selectConversation(uint(id))
				}
			}

			ui.Spacer(c)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(28, 28).Radius(14).Background(t.Accent)
				ui.Column(c).Grow(1).Gap(0).Children(func() {
					ui.Text(c, a.s.Username).FontSize(13).SingleLine()
					ui.Text(c, "已登录").FontSize(11).TextColor(t.TextMuted)
				})
				if ui.Button(c, "退出").Clicked() {
					a.logout()
				}
			})
		})

		// 主区：欢迎页或消息流，底部输入器
		ui.Column(c).Grow(1).FillHeight().MinWidth(0).Children(func() {
			if a.convID == 0 && len(a.msgs) == 0 && !a.loadingC {
				a.welcome(c)
			} else if len(a.msgs) == 0 && !a.loadingC {
				a.emptyConversation(c)
			} else {
				ml := chat.MessageList(c, &a.list, len(a.msgs), chat.MessageListOptions{
					ID:    func(i int) string { return a.msgs[i].id },
					Label: func(i int) string { return a.msgs[i].text },
				}, func(i int) {
					a.message(c, a.msgs[i])
				})
				ml.Element.Grow(1)
				if a.loadingC {
					ui.Row(c).Padding(8, 20).Children(func() {
						ui.Text(c, "加载中…").TextColor(t.TextMuted).FontSize(12)
					})
				}
			}

			ui.Row(c).Gap(10).Padding(12, 20).AlignItems(ui.End).
				BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
				comp := chat.PromptComposer(c, &a.input, chat.PromptComposerOptions{
					Placeholder: "输入一道数学题，例如：画出函数 y = x² − 3x + 1 的图像",
					MaxHeight:   140,
				})
				send := chat.SendButton(c, chat.SendButtonOptions{Streaming: a.streaming(), Shortcut: "Enter"})
				if send.Stopped() {
					if last := a.lastReply(); last != nil {
						last.done = true // 本地停止；后续 SSE 事件会被丢弃
					}
				} else if (send.Sent() || comp.Submitted()) && !a.streaming() {
					a.send(a.input)
				}
			})
		})

		// 右侧：GeoGebra 画板（webview）
		if a.showBoard {
			a.boardPanel(c)
		}
	})
}

// boardPanel 是右侧的 GeoGebra 画板：顶部工具行 + webview（无窗口时显示占位）。
func (a *app) boardPanel(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(470).FillHeight().Shrink(0).Gap(8).Padding(10).
		BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Background(t.Background).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "GeoGebra 画板").Bold().FontSize(13)
			status := "就绪"
			switch {
			case a.ggbView == nil:
				status = "无窗口预览"
			case a.boardBusy:
				status = "应用中…"
			case !a.boardReady:
				status = "加载中…"
			case len(a.boardErrors) > 0:
				status = itoa(len(a.boardErrors)) + " 条失败"
			case a.boardApplied > 0:
				status = "已应用 " + itoa(a.boardApplied) + " 条"
			}
			ui.Text(c, status).FontSize(11).TextColor(t.TextMuted)
			ui.Spacer(c)
			if ui.Button(c, "2D").Label("切到平面视图").Clicked() {
				a.sendToBoard(false, "1", "")
			}
			if ui.Button(c, "3D").Label("切到立体视图").Clicked() {
				a.sendToBoard(false, "T", "")
			}
			if ui.Button(c, "清空").Clicked() {
				a.sendToBoard(true, "", "")
			}
			if ui.Button(c, "收起").Clicked() {
				a.showBoard = false
			}
		})
		if a.ggbView != nil {
			ui.WebView(c, a.ggbView).Grow(1).Radius(8).Border(1, t.Border)
		} else {
			ui.Column(c).Grow(1).Radius(8).Border(1, t.Border).Background(t.Surface).
				AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Text(c, "画板会在应用窗口中加载").TextColor(t.TextMuted).FontSize(12)
				if a.boardApplied > 0 {
					ui.Text(c, "最近一批指令："+itoa(a.boardApplied)+" 条").TextColor(t.TextMuted).FontSize(12)
				}
			})
		}
	})
}

// emptyConversation 是已选会话但没有消息时的空态。
func (a *app) emptyConversation(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		ui.Text(c, "这个对话还没有消息").TextColor(t.TextMuted)
	})
}

func (a *app) lastReply() *uiMsg {
	for i := len(a.msgs) - 1; i >= 0; i-- {
		if a.msgs[i].role == chat.MessageAssistant {
			return a.msgs[i]
		}
	}
	return nil
}

func (a *app) welcome(c *ui.Context) {
	t := c.Theme()
	w := chat.WelcomeScreen(c, chat.WelcomeScreenOptions{
		Greeting: "画一道数学题",
		Subtitle: "输入题目，自动识别类型、提取元素、生成 GeoGebra 指令与交互页面",
		Cards: []chat.CapabilityCard{
			{Title: "平面几何", Description: "三角形、圆与轨迹", Icon: icons.Must("square")},
			{Title: "立体几何", Description: "空间图形与截面", Icon: icons.Must("archive")},
			{Title: "函数图像", Description: "多项式、三角与圆锥曲线", Icon: icons.Must("layout-grid")},
			{Title: "尺规作图", Description: "用指令精确作图", Icon: icons.Must("pencil")},
		},
	})
	if w.Example != "" {
		a.input = w.Example
	}
	_ = t
}

// ---- 单条消息 ----

func (a *app) message(c *ui.Context, m *uiMsg) {
	if m.role == chat.MessageUser {
		chat.MessageBubble(c, chat.MessageUser, chat.MessageBubbleOptions{Name: "我"}, func() {
			ui.Text(c, m.text).Selectable()
		})
		return
	}
	chat.MessageBubble(c, chat.MessageAssistant, chat.MessageBubbleOptions{Name: "GGB 助手", MaxWidth: 680}, func() {
		t := c.Theme()
		if !m.done && m.step != "" {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(7, 7).Radius(4).Background(t.Accent)
				ui.Text(c, m.step+"…").FontSize(12).TextColor(t.TextMuted)
			})
		}
		if m.thinking != "" {
			elapsed := time.Since(m.started)
			if m.started.IsZero() || m.done {
				elapsed = 0 // 历史消息没有开始时间，不显示时长
			}
			chat.ThinkingBlock(c, &m.open, chat.ThinkingBlockOptions{
				Thinking: !m.done,
				Started:  m.started,
				Elapsed:  elapsed,
			}, func() {
				ui.Text(c, m.thinking).Selectable().FontSize(12).TextColor(t.TextMuted)
			})
		}
		if m.errText != "" {
			ui.Text(c, m.errText).Selectable().TextColor(t.Danger).FontSize(13)
		}
		if m.text != "" {
			if m.done {
				markdown.View(c, m.text, &a.md)
			} else {
				markdown.Stream(c, m.text, &a.md)
			}
		}
		if m.ggb != "" {
			chat.CodeBlock(c, m.ggb, chat.CodeBlockOptions{})
		}
		if m.html != "" {
			art := chat.ArtifactCard(c, chat.ArtifactCardOptions{
				Kind:      agent.ArtifactWeb,
				Title:     "GeoGebra 交互页面",
				Detail:    "HTML · 点击在浏览器中打开",
				Streaming: !m.done,
			})
			if art.Changed() {
				a.openArtifact(m.html)
			}
		}
	})
}

func firstRune(s string) string {
	for _, r := range s {
		return string(r)
	}
	return "?"
}

var _ = fmt.Sprintf
