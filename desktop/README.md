# ggb-desktop

ggb_server 的原生桌面客户端：MyGo 自绘 UI（MujicaUI mint 风格组件）+ 内嵌 GeoGebra 画板。
聊天直连**任意 OpenAI 兼容接口**（DeepSeek / 通义 / vLLM / one-api …），不依赖 ggb_server。

## 运行

```sh
cd desktop
go run .
```

首次打开点「设置」：填接口地址（如 `https://api.deepseek.com`）、API Key、模型名。
配置持久化在 `~/Library/Application Support/ggb-desktop/settings.json`（Linux/Windows 在各自 UserConfigDir）。

## 功能

- **直连 OpenAI 格式**：`POST {base}/chat/completions` 流式；`reasoning_content` 进可折叠思考块
- **图片提问**：左下「附图」选本地图片，base64 直发视觉模型（OpenAI vision 格式），气泡内直接预览
- **GeoGebra 画板**：右侧 webview 画板；回复中 ` ```ggb ` 围栏里的指令**自动**逐条 `evalCommand` 画上去，
  3D 指令自动切 3D 视角；也可手动 2D/3D/清空
- **布局**：左右分割线可拖动调宽；侧栏可收起展开；会话本地持久化
  （`~/.../ggb-desktop/history.json`，图片不落盘）

## GeoGebra 画板的加载

- **完全离线（推荐）**：把 [geogebra-dsh](https://github.com/HycJack/geogebra-dsh)
  的 `vendor/GeoGebra`（约 45MB）拷到 `desktop/web/vendor/GeoGebra`
  （已 gitignore），或放 `~/.ggb-desktop/web/vendor/GeoGebra`，或用
  `GGB_GEOGEBRA_ROOT` 指向包含 `vendor/GeoGebra` 的目录
- **在线回退**：本地构建缺失时自动回退官方 CDN（需要网络）

桥接：Go → 页面事件 `ggb-cmds`（`{id, perspective, reset, commands[]}`）；
页面 → Go：`GGBBridge.Ready` / `GGBBridge.Report(id, applied, errors)`。

## 截图

```sh
go test ./...    # 生成 testdata/*.png（欢迎页 / 设置引导 / 会话 / 流式中）
```

## 打包

```sh
go tool mygo build .    # 依赖 go.mod 的 tool 指令；产出 build/
```
