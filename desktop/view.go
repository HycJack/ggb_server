package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

func (a *app) view(c *ui.Context) {
	t := c.Theme()

	// 三栏用原生 ui.Split：分割条由框架托管（可拖动、支持键盘），不再手搓
	rest := func() {
		if a.showBoard {
			ui.Split(c, &a.chatWidth,
				func() { a.chatPane(c, t) },
				func() { a.boardPanel(c, t) },
			).Fill()
		} else {
			a.chatPane(c, t)
		}
	}
	if a.showSidebar {
		ui.Split(c, &a.sidebarWidth,
			func() { a.sidebar(c, t) },
			rest,
		).Fill()
	} else {
		rest()
	}

	a.settingsModal(c, t)
}

// chatPane 是标题行 + 欢迎页/消息流 + 输入器。
func (a *app) chatPane(c *ui.Context, t *ui.Theme) {
	ui.Column(c).FillHeight().MinWidth(0).Children(func() {
		a.chatHeader(c, t)
		switch {
		case len(a.currentMsgs()) == 0:
			a.welcome(c, t)
		default:
			msgs := a.currentMsgs()
			a.list.Key = func(i int) any { return msgs[i].id }
			a.list.FollowEnd = true
			ui.List(c, &a.list, len(msgs), func(i int) {
				a.message(c, t, msgs[i])
			}).Grow(1).Padding(16, 20).Gap(16).Justify(ui.End).Children(func() {
				if len(msgs) == 0 {
					ui.Text(c, "发一条消息开始对话").TextColor(t.TextMuted).Margin(ui.Auto)
				}
			})
		}
		a.composer(c, t)
	})
}

// ---- 侧栏 ----

func (a *app) sidebar(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Width(a.sidebarWidth).FillHeight().Shrink(0).Padding(12).Gap(10).
		Background(t.Surface).BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Children(func() {
		// 品牌行 + 收起
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "GGB 画图助手").Bold().FontSize(14).Grow(1)
			if ui.Button(c, "‹").Label("收起侧栏").Clicked() {
				a.showSidebar = false
			}
		})

		if ui.PrimaryButton(c, "新建对话").Clicked() {
			a.newConversation()
		}

		// 会话列表（分组、相对时间由组件内置）
		ui.Scroll(c).Grow(1).Gap(2).Children(func() {
			for i, cv := range a.convs {
				a.convRow(c, t, i, cv)
			}
		})

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
			if ui.Button(c, "设置").Clicked() {
				a.openSettings()
			}
		})
	})
}

// convRow 是侧栏里的一条会话：整行可点，选中高亮，行尾删除。
func (a *app) convRow(c *ui.Context, t *ui.Theme, i int, cv conversation) {
	selected := cv.ID == a.convID
	row := ui.Row(c.Key(cv.ID)).FillWidth().Padding(8, 10).Radius(8).Gap(6).AlignItems(ui.Center)
	if selected {
		row = row.Background(t.Selection)
	}
	row.Children(func() {
		ui.Column(c).Grow(1).Gap(0).Children(func() {
			ui.Text(c, cv.Title).SingleLine().FontSize(13)
			if !cv.UpdatedAt.IsZero() {
				ui.Text(c, humanTime(cv.UpdatedAt)).FontSize(11).TextColor(t.TextMuted)
			}
		})
		del := ui.Text(c, "✕").TextColor(t.TextMuted).FontSize(12)
		if del.Clicked() {
			a.deleteConversation(cv.ID)
		}
	})
	if row.Clicked() {
		a.selectConversation(cv.ID)
	}
}

func humanTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return "刚刚"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + " 小时前"
	case d < 30*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/24)) + " 天前"
	default:
		return t.Format("01-02")
	}
}

// chatHeader 是主区标题行。
func (a *app) chatHeader(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Padding(10, 16).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		if !a.showSidebar {
			if ui.Button(c, "☰").Label("展开侧栏").Clicked() {
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
func (a *app) settingsModal(c *ui.Context, t *ui.Theme) {
	ui.Modal(c, &a.showSettings, func() {
		ui.Column(c).Width(460).Gap(14).Padding(24, 22).Radius(14).Background(t.Surface).
			BorderWidth(1, 1, 1, 1).BorderColor(t.Border).Children(func() {
			ui.Text(c, "接口设置").FontSize(18).Bold()
			ui.Text(c, "填任意 OpenAI 兼容端点（DeepSeek / 通义 / vLLM / one-api 等）").
				FontSize(12).TextColor(t.TextMuted)

			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, "接口地址").FontSize(12).TextColor(t.TextMuted)
				ui.TextInput(c, &a.draft.APIBase).Placeholder("https://api.deepseek.com")
			})
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, "API Key").FontSize(12).TextColor(t.TextMuted)
				ui.TextInput(c, &a.draft.APIKey).Placeholder("sk-...")
			})
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, "模型").FontSize(12).TextColor(t.TextMuted)
				ui.TextInput(c, &a.draft.Model).Placeholder("deepseek-chat")
			})
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, "系统提示词").FontSize(12).TextColor(t.TextMuted)
				ui.TextInput(c, &a.draft.SystemPrompt)
			})
			ui.Checkbox(c, &a.draft.UseTools, "启用画板工具调用（ReAct，推荐）")
			ui.Checkbox(c, &a.draft.AutoBoard, "自动把 GGB 指令画到画板（围栏兜底）")

			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.showSettings = false
				}
				if ui.PrimaryButton(c, "保存").Clicked() {
					a.s = a.draft // 只有保存才写回真实配置
					a.rebuildClient()
					a.saveAll()
					a.showSettings = false
				}
			})
		})
	})
}

// composer 是底部输入区：附件条 + 输入框 + 发送按钮。
func (a *app) composer(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Gap(0).Shrink(0).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		if a.attachPath != "" || a.attachErr != "" {
			ui.Row(c).Padding(10, 16, 0, 16).Gap(8).AlignItems(ui.Center).Children(func() {
				if a.attachPath != "" {
					name := baseName(a.attachPath)
					label := name
					if a.attachBusy {
						label = name + "（读取中…）"
					}
					ui.Row(c).Padding(4, 10).Radius(8).Background(t.Surface).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "🖼 "+label).FontSize(12)
						if ui.Text(c, "✕").TextColor(t.TextMuted).FontSize(12).Clicked() {
							a.clearAttachment()
						}
					})
				}
				if a.attachErr != "" {
					ui.Text(c, a.attachErr).FontSize(12).TextColor(t.Danger)
				}
			})
		}
		ui.Row(c).Padding(12, 16).Gap(10).AlignItems(ui.End).Children(func() {
			in := ui.TextInput(c, &a.input).Placeholder("输入数学题，或先点「附图」…").Grow(1).AutoFocus()
			if a.streaming() {
				if ui.PrimaryButton(c, "停止").Clicked() {
					if last := a.lastReply(); last != nil {
						last.done = true
					}
				}
			} else {
				send := ui.PrimaryButton(c, "发送").Disabled(a.input == "" || !a.configured())
				if send.Clicked() || in.Submitted() {
					if a.input != "" {
						a.send(a.input)
					}
				}
			}
		})
		// 工具行：附图 + 未配置提示
		ui.Row(c).Padding(0, 16, 10, 16).Gap(10).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, "附图").Label("上传题目图片").Disabled(a.attachBusy).Clicked() {
				a.pickImage()
			}
			ui.Spacer(c)
			if !a.configured() {
				if ui.Text(c, "还没有配置接口，点这里设置").FontSize(12).TextColor(t.Danger).Clicked() {
					a.openSettings()
				}
			}
		})
	})
}

// userMessage 渲染用户消息：图片在上，强调色气泡在下，整体右对齐。
func (a *app) userMessage(c *ui.Context, t *ui.Theme, m *uiMsg) {
	ui.Row(c).Children(func() {
		ui.Spacer(c)
		ui.Column(c).MaxWidth(480).AlignItems(ui.End).Gap(6).Children(func() {
			if m.bitmap != nil {
				// 列表行对图片的测高不稳定，用固定尺寸盒子兜底
				ui.Box(c).Shrink(0).Size(248, 188).Radius(10).Clip().Children(func() {
					ui.Image(c, m.bitmap).Size(240, 180)
				})
			}
			if m.text != "" {
				ui.Text(c, m.text).Selectable().TextColor(t.AccentText).
					Padding(8, 12).Radius(14).Background(t.Accent).
					MaxWidth(480).Label("我 说")
			}
		})
	})
}

func (a *app) lastReply() *uiMsg {
	msgs := a.currentMsgs()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].role == roleAssistant {
			return msgs[i]
		}
	}
	return nil
}

func (a *app) welcome(c *ui.Context, t *ui.Theme) {
	if !a.configured() {
		ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Gap(12).Children(func() {
			ui.Text(c, "先配置一个 OpenAI 兼容接口").FontSize(15)
			if ui.PrimaryButton(c, "打开设置").Clicked() {
				a.openSettings()
			}
		})
		return
	}
	ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Gap(14).Children(func() {
		ui.Text(c, "画一道数学题").FontSize(22).Bold()
		ui.Text(c, "输入题目，或点「附图」上传题目照片；GGB 指令会自动画到右侧画板").
			TextColor(t.TextMuted)
		ui.Grid(c).Columns(2).Gap(10).Children(func() {
			for _, card := range []struct{ title, desc string }{
				{"平面几何", "三角形、圆与轨迹"},
				{"立体几何", "空间图形与截面"},
				{"函数图像", "多项式、三角与圆锥曲线"},
				{"尺规作图", "用指令精确作图"},
			} {
				ui.Box(c).Width(220).Padding(14).Radius(10).Gap(4).Background(t.Surface).Border(1, t.Border).Children(func() {
					ui.Text(c, card.title).Bold()
					ui.Text(c, card.desc).FontSize(12).TextColor(t.TextMuted)
				})
			}
		})
	})
}

// ---- 单条消息 ----

func (a *app) message(c *ui.Context, t *ui.Theme, m *uiMsg) {
	if m.role == roleUser {
		a.userMessage(c, t, m)
		return
	}
	ui.Column(c).MaxWidth(720).Gap(8).Children(func() {
		// 思考块（可折叠）
		if m.thinking != "" {
			label := "思考过程"
			if !m.done {
				label = "思考中…"
			}
			ui.Collapsible(c, label, &m.open, func() {
				ui.Text(c, m.thinking).Selectable().FontSize(12).TextColor(t.TextMuted)
			})
		}
		if len(m.tools) > 0 {
			label := "画板操作（" + strconv.Itoa(len(m.tools)) + "）"
			ui.Collapsible(c, label, &m.toolsOpen, func() {
				ui.Column(c).Gap(6).Children(func() {
					for _, s := range m.tools {
						ui.Column(c).Gap(2).Padding(6, 10).Radius(8).Background(t.Surface).Children(func() {
							ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
								badge := t.Accent
								if s.Failed {
									badge = t.Danger
								}
								ui.Text(c, toolLabel(s.Name)).FontSize(11).Bold().TextColor(badge)
								if s.Args != "" {
									ui.Text(c, s.Args).FontSize(11).TextColor(t.TextMuted).Grow(1)
								}
							})
							if s.Result != "" {
								ui.Text(c, s.Result).FontSize(11).TextColor(t.TextMuted)
							}
						})
					}
				})
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
			if ui.Button(c, "🌐 在浏览器打开 GeoGebra 页面").Clicked() {
				a.openArtifact(m.html)
			}
		}
	})
}

// boardPanel 是右侧的 GeoGebra 画板：顶部工具行 + webview（无窗口时显示占位）。
func (a *app) boardPanel(c *ui.Context, t *ui.Theme) {
	ui.Column(c).FillHeight().Gap(8).Padding(12).
		BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Background(t.Background).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "GeoGebra 画板").Bold().FontSize(13)
			status := "就绪"
			switch {
			case a.ggbView == nil:
				status = "预览"
			case a.boardBusy:
				status = "处理中…"
			case !a.boardReady:
				status = "加载中…"
			case a.boardNote != "":
				status = a.boardNote
			case len(a.boardErrors) > 0:
				status = strconv.Itoa(len(a.boardErrors)) + " 条失败"
			case a.boardObjectCount > 0:
				status = strconv.Itoa(a.boardObjectCount) + " 个对象"
			case a.boardApplied > 0:
				status = "已应用 " + strconv.Itoa(a.boardApplied) + " 条"
			}
			ui.Text(c, status).FontSize(11).TextColor(t.TextMuted).Grow(1).SingleLine()
			if ui.Button(c, "打开").Label("打开 .ggb 文件").Clicked() {
				a.openGGBFile()
			}
			if ui.Button(c, "导出").Label("导出 .ggb 文件").Clicked() {
				a.boardOp("export")
			}
			if ui.Button(c, "2D").Label("切到平面视图").Clicked() {
				a.sendToBoard(false, "AG", "")
			}
			if ui.Button(c, "3D").Label("切到立体视图").Clicked() {
				a.sendToBoard(false, "AT", "")
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
