package main

// GeoGebra 画板的前端资源与 Go↔页面桥接。
//
// 架构参考 geogebra-dsh：嵌入页优先加载完全离线的本地 GeoGebra 构建
// （web/vendor/GeoGebra，见 README），缺失时回退官方 CDN。Go 侧通过
// mygo 事件把成批的 GGB 指令发给页面，页面用 ggbApplet.evalCommand
// 逐条应用并回报结果。

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
)

//go:embed web
var webFS embed.FS

// ggbCmdsEvent 是 Go → 页面的命令事件；负载是 JSON 字符串。
var ggbCmdsEvent = mygo.NewEvent[string]("ggb-cmds")

// boardMsg 是发给嵌入页的命令负载。
type boardMsg struct {
	ID          string   `json:"id"`
	Perspective string   `json:"perspective,omitempty"` // "1"=2D，"T"=3D
	Reset       bool     `json:"reset,omitempty"`
	Commands    []string `json:"commands"`
}

// GGBBridge 绑定给页面回调：画板就绪与指令应用结果。
type GGBBridge struct{ app *app }

// Ready 由嵌入页在 applet 加载完成后调用。
func (g *GGBBridge) Ready(ctx context.Context) {
	a := g.app
	a.win.Update(func() { a.boardReady = true })
}

// Report 由嵌入页在每批指令应用完后调用。
func (g *GGBBridge) Report(ctx context.Context, id string, applied int, errors []string) {
	a := g.app
	a.win.Update(func() {
		a.boardBusy = false
		a.boardApplied = applied
		a.boardErrors = errors
	})
}

// ---- 指令 → 画板 ----

// threeDSignals 是判断题目属于 3D 视角的指令特征。
var threeDSignals = []string{
	"Cube(", "Sphere(", "Cylinder(", "Cone(", "Pyramid(", "Prism(",
	"Tetrahedron(", "Polyhedron(", "PerpendicularPlane(", "Plane(",
	"Rotate(,", "Dilate(,,", "Reflect(,", "Distance(,", "Intersect(,",
}

// inferPerspective 猜测这批指令需要的视角；判不了就留在当前视角（空串）。
func inferPerspective(commands []string) string {
	for _, cmd := range commands {
		for _, sig := range threeDSignals {
			if strings.Contains(cmd, sig) {
				return "T"
			}
		}
	}
	return ""
}

// splitCommands 把一段多行指令文本拆成逐条指令，去掉空行与注释。
func splitCommands(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// sendToBoard 把一批指令发到画板；headless 下只记录状态供截图。
func (a *app) sendToBoard(reset bool, perspective, text string) {
	commands := splitCommands(text)
	if len(commands) == 0 && !reset && perspective == "" {
		return
	}
	a.boardSeq++
	msg := boardMsg{ID: "b" + itoa(a.boardSeq), Perspective: perspective, Reset: reset, Commands: commands}
	a.boardBusy = a.ggbView != nil
	a.boardErrors = nil
	if a.ggbView == nil {
		a.boardApplied = len(commands) // headless：假装全部成功，保证测试确定性
		return
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}
	ggbCmdsEvent.Broadcast(string(payload))
}

// ---- 前端资源服务 ----

// frontendFS 返回 SetFrontend 用的文件系统：嵌入的页面优先，
// 本地 vendor/GeoGebra 存在时叠加进 /web/vendor/** 路径。
func frontendFS() fs.FS {
	if root := geogebraRoot(); root != "" {
		return unionFS{primary: webFS, fallback: os.DirFS(root), prefix: "web/"}
	}
	return webFS
}

// geogebraRoot 找到包含 vendor/GeoGebra 的目录；找不到返回空串。
func geogebraRoot() string {
	probe := func(root string) string {
		if root == "" {
			return ""
		}
		if info, err := os.Stat(filepath.Join(root, "vendor", "GeoGebra", "deployggb.js")); err == nil && !info.IsDir() {
			return root
		}
		return ""
	}
	if p := os.Getenv("GGB_GEOGEBRA_ROOT"); p != "" {
		if r := probe(p); r != "" {
			return r
		}
	}
	if wd, err := os.Getwd(); err == nil {
		if r := probe(filepath.Join(wd, "web")); r != "" {
			return r
		}
	}
	if exe, err := os.Executable(); err == nil {
		if r := probe(filepath.Join(filepath.Dir(exe), "web")); r != "" {
			return r
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		if r := probe(filepath.Join(home, ".ggb-desktop", "web")); r != "" {
			return r
		}
	}
	return ""
}

// unionFS 先查 primary，未命中且带 prefix 的路径转给 fallback。
type unionFS struct {
	primary  fs.FS
	fallback fs.FS
	prefix   string // primary 上的路径前缀，映射到 fallback 时去掉
}

func (u unionFS) Open(name string) (fs.File, error) {
	if f, err := u.primary.Open(name); err == nil {
		return f, nil
	}
	if u.fallback != nil && strings.HasPrefix(name, u.prefix) {
		return u.fallback.Open(strings.TrimPrefix(name, u.prefix))
	}
	return nil, fs.ErrNotExist
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
