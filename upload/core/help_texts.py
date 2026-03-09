from __future__ import annotations


def build_suite_help_ios(host: str) -> str:
    return f"""#!name=Suite Upload Helper
#!desc=自动抓取 Suite 数据并上传到 LunaBot 服务器 ({host})
#!author=Nene-LunaBot
#!system=ios
#!redirect=2
#!mitm=2
#!total=4
#按需修改配置注释

[Script]
# 国服
SCRIPT_cn_suite_1 = type=http-response, requires-body=1, binary-body-mode=1, max-size=100000000, timeout=60, pattern=^https:\\/\\/mkcn-prod-public-60001-1\\.dailygn\\.com\\/api\\/suite\\/user\\/(\\d+)$, script-path=http://{host}/public/scripts/upload.js
SCRIPT_cn_suite_2 = type=http-response, requires-body=1, binary-body-mode=1, max-size=100000000, timeout=60, pattern=^https:\\/\\/mkcn-prod-public-60001-2\\.dailygn\\.com\\/api\\/suite\\/user\\/(\\d+)$, script-path=http://{host}/public/scripts/upload.js

# 日服 (取消注释以启用)
# SCRIPT_jp_suite = type=http-response, requires-body=1, binary-body-mode=1, max-size=100000000, timeout=60, pattern=^https:\\/\\/production-game-api\\.sekai\\.colorfulpalette\\.org\\/api\\/suite\\/user\\/(\\d+)$, script-path=http://{host}/public/scripts/upload.js

[MITM]
hostname=%APPEND% mkcn-prod-public-60001-1.dailygn.com, mkcn-prod-public-60001-2.dailygn.com
"""


def build_mysekai_help_ios(host: str) -> str:
    return f"""#!name=MySekai Upload Helper
#!desc=自动抓取 MySekai 数据并上传到 LunaBot 服务器 ({host})
#!author=Nene-LunaBot
#!system=ios
#!redirect=3
#!mitm=3
#!total=6
#按需修改配置注释

[URL Rewrite]
# 日服 (取消注释以启用)
# ^https:\\/\\/production-game-api\\.sekai\\.colorfulpalette\\.org\\/api\\/user\\/(\\d+)\\/mysekai\\?isForceAllReloadOnlyMysekai\\=(True|False)$ http://{host}/api/jp/user/$1/upload/mysekai 307
# ^https:\\/\\/submit\\.backtrace\\.io\\/  reject

# 国服
^https:\\/\\/mkcn-prod-public-60001-1\\.dailygn\\.com\\/api\\/user\\/(\\d+)\\/mysekai\\?isForceAllReloadOnlyMysekai\\=(True|False)$ http://{host}/api/cn/user/$1/upload/mysekai 307
^https:\\/\\/mkcn-prod-public-60001-2\\.dailygn\\.com\\/api\\/user\\/(\\d+)\\/mysekai\\?isForceAllReloadOnlyMysekai\\=(True|False)$ http://{host}/api/cn/user/$1/upload/mysekai 307
^https:\\/\\/submit\\.backtrace\\.io\\/  reject

[MITM]
# hostname=%APPEND% production-game-api.sekai.colorfulpalette.org, submit.backtrace.io
hostname=%APPEND% mkcn-prod-public-60001-1.dailygn.com, mkcn-prod-public-60001-2.dailygn.com, submit.backtrace.io
"""


def build_mysekai_help_android(host: str) -> str:
    return f"""# 虚拟机抓包上传配置指北

Android 使用与 haruki-proxy 类似的原理，具体方法可参考 haruki-proxy 教程
将脚本替换成我所提供的内容
下载启动脚本命令：
wget https://{host}/upload/download/catcher.sh
下载停止脚本命令：
wget https://{host}/upload/download/kill-catcher.sh
下载卸载脚本命令：
wget https://{host}/upload/download/uninstall-catcher.sh
初次使用流程：
1. 在 MT 管理器中，打开终端后 cd 到你的主目录（一般你打开终端后下方会有一个命令，点击确定）
2. 在终端中运行 wget https://{host}/upload/download/catcher.sh，找到该文件点按，设置选择以扩展包环境执行，以root权限执行，点击执行
3. 下载完后如自动重启失败则手动重启虚拟机
使用后需要关闭进程必须使用停止脚本，初次下载之后都直接执行，下载命令见上，如如果后续报错如网络错误或地址已经存在，请先确认是否按要求关闭进程
抓包时直接执行 catcher.sh 即可，然后运行游戏，所提取数据仅供Nenebot使用

## 与harukiproxy不同之处
1. 如果没有harukiproxy证书，注释掉config中指向harukiproxy证书的两行，安装catcher证书后需要重启虚拟机，可在完成后一起重启
2. 如果设置代理失败则需要手动设置代理，在设置中的wifi处长按当前wifi，设置为手动代理，主机名设置为127.0.0.1,端口设置为888，修改完后重启虚拟机
## 注意事项
1. CA证书优先使用haruki-proxy所安装的证书
2. 关闭进程请使用下面的脚本，仅关闭终端无法关闭进程
"""
