# Site / Sekai / Apply 迁移版

`upload_v2` 是对现有 upload 服务的旁路迁移版，不直接修改原 `upload/`，用于把现有职责收口为更稳定的业务边界。

## 当前边界

- `site_root`
  - 负责根入口 `/`
  - 当前先跳转到 `/upload/help`

- `sekai`
  - `web`：`/upload/...` 页面、帮助、下载、状态
  - `ingest_api`：网页上传、iOS 上传、代理上传、MSR、绑定查询
  - `bot_api`：给 lunabot 的 suite / mysekai 数据读取接口

- `apply`
  - 群聊申请页面、审核页面与 `/api/apply/...`

## 当前目标

- 删除旧的无前缀 upload 页面路由依赖
- 保留 `/api/...` 作为 sekai 与 lunabot 的统一接口面
- 让 `apply` 保持独立 feature
- 为后续主帮助页和项目改名预留站点壳层
- sekai 页面资源已收口到 `sekai/pages` 与 `sekai/static`

## 关联组件

- 依赖同级 `lunabot` 中的绑定库与用户数据目录
- 依赖同级 `catcher` 中的 Android 可执行文件、配置和脚本
