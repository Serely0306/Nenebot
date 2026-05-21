# Catcher

`catcher` 是 Nenebot 的 Android 侧抓包组件。它通过 HTTPS MITM 拦截 Project Sekai 指定接口，按配置抓取 Suite 与 MySekai 数据，解密后上传到 `upload` 服务，或在本地保留结果与原始响应。

## 当前能力

- 支持 JP / CN 区服接口识别
- 支持按配置抓取 Suite 和 MySekai 响应
- 可自动将 MySekai 请求改写为强制刷新模式
- 自动生成 CA
- 在 Android7 虚拟机 Root 环境下自动安装证书、设置代理并清理环境

Android 使用与配置说明见 `docs/TUTORIAL.md`。
