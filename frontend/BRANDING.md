# 狗站 / 88dog 品牌定制

品牌名统一为 `88dog`，域名为 `88888888.dog`。使用已确认的暖橙＋奶油白、深咖字标和侧身奔跑犬剪影，与充值站保持一致。只调整视觉和品牌显示，不扩展吉祥物互动或动画功能。

## 定制边界

- 沿用上游 Vue 页面、布局、路由、接口和业务逻辑，不新增依赖。
- `frontend/src/utils/branding.ts` 集中保存品牌默认值。空站名、上游默认 `Sub2API` 及旧默认「狗站」显示为 `88dog`；空副标题及上游默认介绍显示为 `88dog · 你的 AI 接口小站`。
- 后台配置的其他站名、副标题、Logo 和自定义首页继续有效；上述显示回退不写数据库、不改公开 API 响应或注入配置。
- 首页、登录布局、注册、邮箱验证及浏览器标题使用统一默认值。技术说明、协议标识、存储键及上游项目署名保留原样。
- `frontend/tailwind.config.js` 统一维护暖橙 `primary` 色阶及浅色暖灰背景。完整暖灰色阶、深咖深色模式与暖橙主色配套；品牌强调橙 `#E86F3A`，白字主按钮用较深的 `#BF4B1E` 保持对比度。成功、警告、错误及支付品牌色不变。
- `frontend/public/branding/88dog-wordmark.png` 为内置生图工具生成、用户确认的透明组合字标；`frontend/public/logo.svg` 是同方向犬剪影的原生 SVG 小尺寸图标，用于侧栏和 favicon。旧柯基素材仅作历史保留，不再作为默认标志。无外部图片请求，无动画。
- `frontend/index.html` 保存无 JavaScript 时的默认标题及奶油白浏览器主题色。后端仍优先注入数据库的站点名称，前端会统一处理上述旧默认值；本次不改数据库配置或公开 API 响应。
- OpenDesign 的 Family 参考用于轻量表面、8px 间距和克制交互；配色与犬剪影来自用户确认的 88dog 独立方案。共享按钮去除装饰性渐变和光晕，保持原业务状态与支付标识语义。


## 分支与上游同步

当前上游为 `ranxi2001/sub2api`，远程名 `upstream`；生产来源分支实际名称是 `production`。狗站定制放在 `ray/gouzhan-brand`，不要将上游 `production` 改成品牌专用分支。

后续发布流程：在狗站定制分支合入 `upstream/production`，解决冲突并完成验证后，从合并后的代码构建产物，再交给部署流程。提交、合并、推送和生产发布按当次授权执行。不要直接构建上游分支覆盖狗站版本。

优先检查品牌工具、主题配置、默认 Logo，以及调用品牌工具的首页、认证页和标题逻辑。保持这些调用局部化，避免逐页替换业务文案。不要在合并冲突时整文件覆盖上述入口。

## 验证与构建

使用仓库 Dockerfile / CI 对齐的 pnpm 9，并保持锁文件不变：

```sh
cd frontend
npx --yes pnpm@9.15.9 install --frozen-lockfile --ignore-scripts
npx --yes pnpm@9.15.9 exec vitest run src/utils/__tests__/branding.spec.ts src/router/__tests__/title.spec.ts src/stores/__tests__/app.spec.ts src/views/__tests__/HomeView.compact.spec.ts src/views/auth/__tests__/RegisterView.spec.ts src/views/auth/__tests__/EmailVerifyView.spec.ts
npx --yes pnpm@9.15.9 exec eslint src/utils/branding.ts src/router/title.ts src/stores/app.ts src/views/HomeView.vue src/components/layout/AuthLayout.vue src/views/auth/RegisterView.vue src/views/auth/EmailVerifyView.vue tailwind.config.js
```

依赖安装跳过生命周期脚本；如本机平台缺少原生工具，先定向检查对应依赖，不要因此重写锁文件。

前端正式构建包含国际化完整性检查、TypeScript 检查和 Vite 打包。遵守工作区产物约定，输出到当前仓库的同级 `release/`，不要使用默认的仓库内输出目录：

```sh
npx --yes pnpm@9.15.9 run build --outDir ../../release/sub2api-ranxi2001/<release-id>/dist
```

手工检查首页和登录页在桌面、手机、浅色与深色模式下的站名、标识、主按钮和布局；检查后台自定义品牌仍覆盖默认值。生产后端及邮件模板沿用后台站点设置，本次前端改造不替代生产站点配置。
