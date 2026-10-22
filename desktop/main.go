// ggb-desktop：ggb_server 的原生桌面客户端（MyGo + MujicaUI mint 风格）。
//
//	go run .          # 连接 http://localhost:8080 的 ggb_server
//	go test ./...     # 无头渲染截图（testdata/*.png）
package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	a := newApp()
	mygo.SetFrontend(frontendFS())
	mygo.Bind(&GGBBridge{a})
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:   "GGB 数学画图助手",
			Width:   1240,
			Height:  800,
			Content: ui.View(a.view),
		})
		// GeoGebra 画板的 webview；ui.WebView 按布局放置它
		if v, err := a.win.NewWebView(mygo.WebViewOptions{URL: "/web/geogebra.html"}); err == nil {
			a.ggbView = v
		} else {
			log.Println("geogebra webview:", err)
		}
		if a.logged {
			a.refreshConversations()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
