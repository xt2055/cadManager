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

## 数据管理器

业务数据统一通过 `src/services/data-manager` 访问，不在页面中区分 API 或本地保存方式。

- 开发阶段默认使用 JSON Provider。
- Tauri 开发环境将数据保存到应用数据目录下的 `data-document.json`。
- 浏览器执行 `bun dev` 时使用 `localStorage` 保存 JSON 字符串作为降级方案。
- 生产阶段默认使用 API Provider，可通过 `VITE_DATA_PROVIDER` 和 `VITE_API_BASE_URL` 覆盖。

复制 `.env.example` 为本地环境文件后，可配置：

```env
VITE_DATA_PROVIDER=json
VITE_API_BASE_URL=/api
```

当前统一文档接口为：

- `GET /data/document`：加载业务数据文档。
- `PUT /data/document`：保存完整业务数据文档。
