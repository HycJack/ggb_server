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
	"encoding/base64"
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

// boardResult 是画板回调的汇总结果，经 channel 送回等待中的 boardCall。
type boardResult struct {
	applied int
	errors  []string
	count   int
	names   []string
	b64     string
	image   string // 画板截图（data URL）
	note    string
}

// boardMsg 是发给嵌入页的负载；op 缺省为 "run"（执行指令）。
type boardMsg struct {
	ID          string   `json:"id"`
	Op          string   `json:"op,omitempty"`          // run | export | read | open
	Perspective string   `json:"perspective,omitempty"` // "AG"=代数+图形，"AT"=代数+3D
	Reset       bool     `json:"reset,omitempty"`
	Commands    []string `json:"commands"`
	Base64      string   `json:"base64,omitempty"` // op=open 时的 .ggb 内容
}

// GGBBridge 绑定给页面回调：画板就绪与指令应用结果。
type GGBBridge struct{ app *app }

// Ready 由嵌入页在 applet 加载完成后调用。
func (g *GGBBridge) Ready(ctx context.Context) {
	a := g.app
	a.mutate(func() { a.boardReady = true })
}

// Report 由嵌入页在每批指令应用完后调用。
func (g *GGBBridge) Report(ctx context.Context, id string, applied int, errors []string) {
	a := g.app
	a.deliverBoard(id, boardResult{applied: applied, errors: errors})
	a.mutate(func() {
		a.boardBusy = false
		a.boardApplied = applied
		a.boardErrors = errors
	})
}

// State 由嵌入页回报画板对象数与名字清单（count=-1 表示出错，names[0] 是错误信息）。
func (g *GGBBridge) State(ctx context.Context, id string, count int, namesJSON string) {
	a := g.app
	var names []string
	json.Unmarshal([]byte(namesJSON), &names)
	a.deliverBoard(id, boardResult{count: count, names: names})
	a.mutate(func() {
		a.boardObjectCount = count
		a.boardObjects = names
		if count < 0 && len(names) > 0 {
			a.boardNote = "画板操作失败：" + names[0]
		}
	})
}

// Image 由嵌入页回调画板截图（data URL）。画完后用来确认画得对不对。
func (g *GGBBridge) Image(ctx context.Context, id, dataURL string) {
	a := g.app
	a.deliverBoard(id, boardResult{image: dataURL})
	a.mutate(func() {
		a.boardImageURL = dataURL
		a.boardBitmapCache = nil // 换了图就重新解码
	})
}

// Export 由嵌入页回调 .ggb 的 base64。ReAct 工具调用路径下结果送回等待者；
// 面板按钮路径弹保存对话框写文件。
func (g *GGBBridge) Export(ctx context.Context, id, b64 string) {
	a := g.app
	if a.deliverBoard(id, boardResult{b64: b64}) {
		return
	}
	if b64 == "" {
		a.mutate(func() { a.boardNote = "导出失败：画板没有返回内容" })
		return
	}
	a.boardBusy = true
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:      a.win,
			Title:       "导出 GeoGebra 文件",
			DefaultPath: "画板.ggb",
		})
		note := ""
		if err == nil && path != "" {
			data, decErr := base64.StdEncoding.DecodeString(b64)
			if decErr != nil {
				note = "导出失败：" + decErr.Error()
			} else if werr := os.WriteFile(path, data, 0o644); werr != nil {
				note = "导出失败：" + werr.Error()
			} else {
				note = "已导出 " + filepath.Base(path)
			}
		}
		a.mutate(func() {
			a.boardBusy = false
			a.boardNote = note
		})
	}()
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
				return "AT"
			}
		}
	}
	return ""
}

// inferPerspective 猜测这批指令需要的视角；判不了就留在当前视角（空串）。
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
