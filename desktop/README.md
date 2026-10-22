# ggb-desktop

ggb_server 的原生桌面客户端：MyGo 自绘 UI（MujicaUI mint 风格组件）+ 内嵌 GeoGebra 画板。

## 运行

```sh
cd desktop
go run .          # 默认连接 http://localhost:8080，可在登录页改服务器地址
```

登录用的账号来自 ggb_server 的 `/api/register`（网页端或接口注册）。

## GeoGebra 画板

右侧画板是一个 webview，加载 `web/geogebra.html`：

- **完全离线（推荐）**：把 [geogebra-dsh](https://github.com/HycJack/geogebra-dsh)
  仓库里的 `vendor/GeoGebra`（约 45MB）拷到 `desktop/web/vendor/GeoGebra`，
  或放到 `~/.ggb-desktop/web/vendor/GeoGebra`，或用环境变量
  `GGB_GEOGEBRA_ROOT` 指向包含 `vendor/GeoGebra` 的目录。
  `web/vendor` 已在 .gitignore 中，不会提交。
- **在线回退**：找不到本地构建时自动回退官方 CDN（需要网络）。

Go 与画板的桥接（参考 geogebra-dsh 的 host/client 结构）：

- Go → 页面：`ggb-cmds` 事件，负载 `{id, perspective, reset, commands[]}`
- 页面 → Go：`GGBBridge.Ready` / `GGBBridge.Report(id, applied, errors)`
- AI 回复中的 `ggb_commands` 步骤输出会**自动**逐条 `evalCommand` 到画板；
  3D 指令（Cube/Sphere/Cylinder 等）自动切 3D 视角。

## 截图

```sh
go test ./...    # 生成 testdata/*.png（登录页 / 欢迎页 / 会话 / 流式中）
```

## 打包

```sh
go tool mygo build .    # 依赖 go.mod 的 tool 指令；产出 build/
```
