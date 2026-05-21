# MySekai / Suite 数据上传服务

`upload` 是 Nenebot 的数据接入层，用于接收 MySekai 与 Suite 数据、完成标准化处理，并向 Bot 与前端页面提供统一读取接口。

## 当前职责

- 接收网页上传、代理转发上传、iOS 分片上传
- 解密二进制响应并归一化保存到 `lunabot/data/sekai/user_data`
- 提供绑定查询、为lunabot本地数据读取与 Suite 的 本地和Haruki ToolBox 并行接口查询（非公开接口）
- 提供 MSR 图片渲染接口
- 提供帮助页面、iOS 上传脚本以及 Android `catcher` 下载入口

## 关联组件

- 依赖同级 `lunabot` 中的绑定库与用户数据目录
- 依赖同级 `catcher` 中的 Android 可执行文件、配置和脚本
