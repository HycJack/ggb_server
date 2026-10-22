package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/HycJack/MujicaUI/agent"
	"github.com/HycJack/MujicaUI/chat"
	"github.com/HycJack/MujicaUI/core"
	"github.com/HycJack/MujicaUI/icons"
	"github.com/HycJack/MujicaUI/input"
	"github.com/HycJack/MujicaUI/theme"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

func (a *app) view(c *ui.Context) {
	core.Use(c, core.Settings{Mode: core.System, Locale: core.ZhCN})

	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		if a.showSidebar {
			a.sidebar(c)
			a.splitterV(c, &a.sidebarWidth, &a.sidebarDrag, 180, 420)
		}

		// 主区：标题行 + 欢迎页/消息流 + 输入器
		ui.Column(c).Grow(1).FillHeight().MinWidth(0).Children(func() {
			a.chatHeader(c)
			switch {
			case len(a.currentMsgs()) == 0:
				a.welcome(c)
			default:
				msgs := a.currentMsgs()
				ml := chat.MessageList(c, &a.list, len(msgs), chat.MessageListOptions{
					ID:    func(i int) string { return msgs[i].id },
					Label: func(i int) string { return msgs[i].text },
				}, func(i int) {
					a.message(c, msgs[i])
				})
				ml.Element.Grow(1)
			}
			a.composer(c)
		})

		if a.showBoard {
			a.splitterV(c, &a.boardWidth, &a.boardDrag, 300, 680)
			a.boardPanel(c)
		}
	})

	a.settingsModal(c)
}

// splitterV 是一条可拖动的竖直分割线：拖动改 base 宽度，松手收拢偏移。
func (a *app) splitterV(c *ui.Context, base *float32, offset *ui.Point, min, max float32) {
	t := c.Theme()
	var handle ui.Element
	ui.Row(c).Width(6).FillHeight().Shrink(0).AlignItems(ui.Center).Children(func() {
		handle = ui.Box(c).Size(3, 40).Radius(2).Background(t.Border)
	})
	handle.Draggable(offset, ui.DraggableOptions{Axis: ui.Horizontal})
	if handle.Dragging() {
		*base = clampF(*base+offset.X, min, max)
	} else if offset.X != 0 {
		*base = clampF(*base+offset.X, min, max)
		*offset = ui.Point{}
	}
}

func clampF(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ---- 侧栏 ----

func (a *app) sidebar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(a.sidebarWidth).FillHeight().Shrink(0).Padding(12).Gap(10).
		Background(t.Background).BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Children(func() {
		// 品牌行 + 收起按钮
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.Must("layout-grid")).Size(20, 20).TextColor(t.Accent)
			ui.Text(c, "GGB 画图助手").Font(theme.SerifFont).Bold().FontSize(14).Grow(1)
			if core.IconAction(c, icons.Must("panel-left"), "收起侧栏").Clicked() {
				a.showSidebar = false
			}
		})

		ui.PrimaryButton(c, "新建对话")

		// 会话列表（分组、相对时间由组件内置）
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

		// 底部：画板开关 + 设置
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			board := ui.Button(c, "画板")
			if a.showBoard {
				board = board.Label("收起画板")
			}
			if board.Clicked() {
				a.showBoard = !a.showBoard
			}
			ui.Spacer(c)
			if core.IconAction(c, icons.Must("settings"), "设置").Clicked() {
				a.showSettings = true
			}
		})
	})
}

// chatHeader 是主区标题行。
func (a *app) chatHeader(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Padding(10, 16).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		if !a.showSidebar {
			if core.IconAction(c, icons.Must("panel-left"), "展开侧栏").Clicked() {
				a.showSidebar = true
			}
		}
		ui.Text(c, a.currentTitle()).Bold().SingleLine().Grow(1)
		if a.streaming() {
			ui.Text(c, "生成中…").FontSize(12).TextColor(t.TextMuted)
		}
	})
}

// settingsModal 是接口配置弹窗。
func (a *app) settingsModal(c *ui.Context) {
	ui.Modal(c, &a.showSettings, func() {
		t := c.Theme()
		ui.Column(c).Width(460).Gap(14).Padding(24, 22).Radius(14).Background(t.Surface).
			BorderWidth(1, 1, 1, 1).BorderColor(t.Border).Children(func() {
			ui.Text(c, "接口设置").Font(theme.SerifFont).FontSize(18).Bold()
			ui.Text(c, "填任意 OpenAI 兼容端点（DeepSeek / 通义 / vLLM / one-api 等）").
				FontSize(12).TextColor(t.TextMuted)

			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, "接口地址").FontSize(12).TextColor(t.TextMuted)
				ui.TextInput(c, &a.s.APIBase).Placeholder("https://api.deepseek.com")
			})
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, "API Key").FontSize(12).TextColor(t.TextMuted)
				ui.TextInput(c, &a.s.APIKey).Placeholder("sk-...")
			})
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, "模型").FontSize(12).TextColor(t.TextMuted)
				ui.TextInput(c, &a.s.Model).Placeholder("deepseek-chat")
			})
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, "系统提示词").FontSize(12).TextColor(t.TextMuted)
				ui.TextInput(c, &a.s.SystemPrompt)
			})
			input.Checkbox(c, &a.s.AutoBoard, "自动把 GGB 指令画到画板", input.CheckboxOptions{})

			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.showSettings = false
				}
				if ui.PrimaryButton(c, "保存").Clicked() {
					a.rebuildClient()
					a.saveAll()
					a.showSettings = false
				}
			})
		})
	})
}

// composer 是底部输入区：附件条 + 填满宽度的输入框 + 发送按钮。
func (a *app) composer(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Gap(0).Shrink(0).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		ui.Row(c).Padding(12, 16).AlignItems(ui.End).Gap(10).Children(func() {
			comp := chat.PromptComposer(c, &a.input, chat.PromptComposerOptions{
				Placeholder: "输入数学题，或点「附图」上传题目照片…",
				MaxHeight:   130,
				Attachments: func() {
					if a.attachPath == "" {
						return
					}
					chip := chat.AttachmentChip(c, chat.Attachment{
						Name:      baseName(a.attachPath),
						Type:      "image",
						Size:      a.attachSize,
						Uploading: a.attachBusy,
						Err:       errOrNil(a.attachErr),
					})
					if chip.Removed() {
						a.clearAttachment()
					}
				},
				Tools: func() {
					if ui.Button(c, "附图").Label("上传题目图片").Disabled(a.attachBusy).Clicked() {
						a.pickImage()
					}
				},
			})
			comp.Element.Grow(1) // 输入框填满剩余宽度
			send := chat.SendButton(c, chat.SendButtonOptions{Streaming: a.streaming(), Shortcut: "Enter"})
			if send.Stopped() {
				if last := a.lastReply(); last != nil {
					last.done = true
				}
			} else if (send.Sent() || comp.Submitted()) && !a.streaming() {
				a.send(a.input)
			}
		})
		if a.attachErr != "" {
			ui.Row(c).Padding(0, 16, 10, 16).Children(func() {
				ui.Text(c, "图片读取失败："+a.attachErr).FontSize(12).TextColor(t.Danger)
			})
		}
		if !a.configured() {
			ui.Row(c).Padding(0, 16, 10, 16).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "还没有配置接口，").FontSize(12).TextColor(t.Danger)
				if ui.Text(c, "点这里设置").FontSize(12).TextColor(t.Accent).Clicked() {
					a.showSettings = true
				}
			})
		}
	})
}

// userMessage 渲染用户消息：图片在上，强调色气泡在下，整体右对齐。
func (a *app) userMessage(c *ui.Context, m *uiMsg) {
	t := c.Theme()
	ui.Row(c).Children(func() {
		ui.Spacer(c)
		ui.Column(c).MaxWidth(460).AlignItems(ui.End).Gap(6).Children(func() {
			if m.bitmap != nil {
				// 列表行对图片的测高不稳定，用固定尺寸盒子兜底
				ui.Box(c).Shrink(0).Size(248, 188).Radius(10).Clip().Children(func() {
					ui.Image(c, m.bitmap).Size(240, 180)
				})
			}
			if m.text != "" {
				ui.Text(c, m.text).Selectable().TextColor(t.AccentText).
					Padding(8, 12).Radius(14).Background(t.Accent).
					MaxWidth(460).Label("我 说")
			}
		})
	})
}

func (a *app) lastReply() *uiMsg {
	msgs := a.currentMsgs()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].role == chat.MessageAssistant {
			return msgs[i]
		}
	}
	return nil
}

func (a *app) welcome(c *ui.Context) {
	t := c.Theme()
	if !a.configured() {
		// 未配置接口：引导去设置
		ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Gap(12).Children(func() {
			ui.Icon(c, icons.Must("settings")).Size(36, 36).TextColor(t.TextMuted)
			ui.Text(c, "先配置一个 OpenAI 兼容接口").FontSize(15)
			if ui.PrimaryButton(c, "打开设置").Clicked() {
				a.showSettings = true
			}
		})
		return
	}
	chat.WelcomeScreen(c, chat.WelcomeScreenOptions{
		Greeting: "画一道数学题",
		Subtitle: "输入题目，或点「附图」上传题目照片；GGB 指令会自动画到右侧画板",
		Cards: []chat.CapabilityCard{
			{Title: "平面几何", Description: "三角形、圆与轨迹", Icon: icons.Must("square")},
			{Title: "立体几何", Description: "空间图形与截面", Icon: icons.Must("archive")},
			{Title: "函数图像", Description: "多项式、三角与圆锥曲线", Icon: icons.Must("layout-grid")},
			{Title: "尺规作图", Description: "用指令精确作图", Icon: icons.Must("pencil")},
		},
	})
}

// ---- 单条消息 ----

func (a *app) message(c *ui.Context, m *uiMsg) {
	if m.role == chat.MessageUser {
		a.userMessage(c, m)
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
				elapsed = 0
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

// boardPanel 是右侧的 GeoGebra 画板：顶部工具行 + webview（无窗口时显示占位）。
func (a *app) boardPanel(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(a.boardWidth).FillHeight().Shrink(0).Gap(8).Padding(12).
		BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Background(t.Background).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.Must("layout-grid")).Size(15, 15).TextColor(t.TextMuted)
			ui.Text(c, "GeoGebra 画板").Bold().FontSize(13)
			status := "就绪"
			switch {
			case a.ggbView == nil:
				status = "预览"
			case a.boardBusy:
				status = "应用中…"
			case !a.boardReady:
				status = "加载中…"
			case len(a.boardErrors) > 0:
				status = strconv.Itoa(len(a.boardErrors)) + " 条失败"
			case a.boardApplied > 0:
				status = "已应用 " + strconv.Itoa(a.boardApplied) + " 条"
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
			ui.WebView(c, a.ggbView).Grow(1).Radius(10).Border(1, t.Border)
		} else {
			ui.Column(c).Grow(1).Radius(10).Border(1, t.Border).Background(t.Surface).
				AlignItems(ui.Center).Justify(ui.Center).Gap(8).Children(func() {
				ui.Icon(c, icons.Must("layout-grid")).Size(30, 30).TextColor(t.TextMuted)
				ui.Text(c, "画板将在应用窗口中加载").TextColor(t.TextMuted).FontSize(13)
				ui.Text(c, "回复中的 GGB 指令会自动画上去").TextColor(t.TextMuted).FontSize(12)
				if len(a.boardErrors) > 0 {
					ui.Text(c, "失败的指令："+strconv.Itoa(len(a.boardErrors))+" 条").TextColor(t.Danger).FontSize(12)
				} else if a.boardApplied > 0 {
					ui.Text(c, "最近一批指令："+strconv.Itoa(a.boardApplied)+" 条").TextColor(t.TextMuted).FontSize(12)
				}
			})
		}
	})
}

func errOrNil(s string) error {
	if s == "" {
		return nil
	}
	return fmt.Errorf("%s", s)
}

func baseName(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[i+1:]
		}
	}
	return p
}
