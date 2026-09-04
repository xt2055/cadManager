# cad

This template should help get you started developing with Vue 3 in Vite.

## Recommended IDE Setup

[VS Code](https://code.visualstudio.com/) + [Vue (Official)](https://marketplace.visualstudio.com/items?itemName=Vue.volar) (and disable Vetur).

## Recommended Browser Setup

- Chromium-based browsers (Chrome, Edge, Brave, etc.):
  - [Vue.js devtools](https://chromewebstore.google.com/detail/vuejs-devtools/nhdogjmejiglipccpnnnanhbledajbpd)
  - [Turn on Custom Object Formatter in Chrome DevTools](http://bit.ly/object-formatters)
- Firefox:
  - [Vue.js devtools](https://addons.mozilla.org/en-US/firefox/addon/vue-js-devtools/)
  - [Turn on Custom Object Formatter in Firefox DevTools](https://fxdx.dev/firefox-devtools-custom-object-formatters/)

## Type Support for `.vue` Imports in TS

TypeScript cannot handle type information for `.vue` imports by default, so we replace the `tsc` CLI with `vue-tsc` for type checking. In editors, we need [Volar](https://marketplace.visualstudio.com/items?itemName=Vue.volar) to make the TypeScript language service aware of `.vue` types.

## Customize configuration

See [Vite Configuration Reference](https://vite.dev/config/).

## Project Setup

```sh
bun install
```

### Compile and Hot-Reload for Development

```sh
bun dev
```

### Type-Check, Compile and Minify for Production

```sh
bun run build
```

## 数据访问

前端只使用 Go 服务端 API。页面通过 `src/app/container.ts` 注入按能力拆分的 Application Service，服务端 API 客户端位于 `src/services/api`；不再提供本地 JSON 数据、整表保存或运行模式切换。

复制 `.env.example` 为本地环境文件后，可配置。连接本机 Go 后端时：

```env
VITE_API_BASE_URL=http://127.0.0.1:8080/api
```

如果使用 Tauri 桌面客户端，Go 后端必须先启动在 `127.0.0.1:8080`；如果使用 Vite 开发服务器，也会通过该地址请求 API。

业务读取和写入使用最终资源接口，例如图纸、结构、属性、关系、附件、上传会话和版本 API；旧 `/api/data/*` 整表接口已删除。

客户端更新检查接口为：

- `GET /updates/latest?current_version=0.1.0&platform=windows-x86_64`：获取最新版本和更新说明。
