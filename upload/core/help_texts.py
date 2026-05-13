from __future__ import annotations


def _build_upload_script_url(host: str, region: str, data_type: str) -> str:
    return f"https://{host}/upload/public/scripts/upload.js?region={region}&data_type={data_type}"


def _build_ios_upload_api_url(host: str, region: str, data_type: str) -> str:
    return f"https://{host}/api/ios/upload?region={region}&data_type={data_type}"


def build_suite_help_ios(host: str) -> str:
    return f"""#!name=Suite Upload Helper
#!desc=自动抓取 Suite 数据并上传到 LunaBot 服务器 ({host})
#!author=Nene-LunaBot
#!system=ios
#!redirect=0
#!mitm=2
#!total=3
# 按需修改配置注释

[Script]
# 国服
SCRIPT_cn_suite_1 = type=http-response, requires-body=1, binary-body-mode=1, max-size=100000000, timeout=60, pattern=^https:\\/\\/mkcn-prod-public-60001-1\\.dailygn\\.com\\/api\\/suite\\/user\\/(\\d+)(\\?isLogin=true)?$, script-path={_build_upload_script_url(host, "cn", "suite")}
SCRIPT_cn_suite_2 = type=http-response, requires-body=1, binary-body-mode=1, max-size=100000000, timeout=60, pattern=^https:\\/\\/mkcn-prod-public-60001-2\\.dailygn\\.com\\/api\\/suite\\/user\\/(\\d+)(\\?isLogin=true)?$, script-path={_build_upload_script_url(host, "cn", "suite")}

# 日服 (取消注释以启用)
# SCRIPT_jp_suite = type=http-response, requires-body=1, binary-body-mode=1, max-size=100000000, timeout=60, pattern=^https:\\/\\/production-game-api\\.sekai\\.colorfulpalette\\.org\\/api\\/suite\\/user\\/(\\d+)(\\?isLogin=true)?$, script-path={_build_upload_script_url(host, "jp", "suite")}

[MITM]
hostname=%APPEND% mkcn-prod-public-60001-1.dailygn.com, mkcn-prod-public-60001-2.dailygn.com
"""


def build_all_help_ios(host: str) -> str:
    return f"""#!name=Sekai Upload Helper
#!desc=自动抓取 Suite + 日服 MySekai 数据并上传到 LunaBot 服务器 ({host})
#!author=Nene-LunaBot
#!system=ios
#!redirect=3
#!mitm=3
#!total=6
# 按需修改配置注释

[URL Rewrite]
# 日服 MySekai
^https:\\/\\/production-game-api\\.sekai\\.colorfulpalette\\.org\\/api\\/user\\/(\\d+)\\/mysekai\\?isForceAllReloadOnlyMysekai=False$ https://production-game-api.sekai.colorfulpalette.org/api/user/$1/mysekai?isForceAllReloadOnlyMysekai=True 307
^https:\\/\\/submit\\.backtrace\\.io\\/ reject

[Script]
# 国服 Suite
SCRIPT_cn_suite_1 = type=http-response, requires-body=1, binary-body-mode=1, max-size=100000000, timeout=60, pattern=^https:\\/\\/mkcn-prod-public-60001-1\\.dailygn\\.com\\/api\\/suite\\/user\\/(\\d+)(\\?isLogin=true)?$, script-path={_build_upload_script_url(host, "cn", "suite")}
SCRIPT_cn_suite_2 = type=http-response, requires-body=1, binary-body-mode=1, max-size=100000000, timeout=60, pattern=^https:\\/\\/mkcn-prod-public-60001-2\\.dailygn\\.com\\/api\\/suite\\/user\\/(\\d+)(\\?isLogin=true)?$, script-path={_build_upload_script_url(host, "cn", "suite")}

# 日服 (按需取消注释 Suite；MySekai 已保留)
# SCRIPT_jp_suite = type=http-response, requires-body=1, binary-body-mode=1, max-size=100000000, timeout=60, pattern=^https:\\/\\/production-game-api\\.sekai\\.colorfulpalette\\.org\\/api\\/suite\\/user\\/(\\d+)(\\?isLogin=true)?$, script-path={_build_upload_script_url(host, "jp", "suite")}
SCRIPT_jp_mysekai = type=http-response, requires-body=1, binary-body-mode=1, max-size=100000000, timeout=60, pattern=^https:\\/\\/production-game-api\\.sekai\\.colorfulpalette\\.org\\/api\\/user\\/(\\d+)\\/mysekai\\?isForceAllReloadOnlyMysekai=True, script-path={_build_upload_script_url(host, "jp", "mysekai")}

[MITM]
hostname=%APPEND% mkcn-prod-public-60001-1.dailygn.com, mkcn-prod-public-60001-2.dailygn.com, production-game-api.sekai.colorfulpalette.org, submit.backtrace.io
"""


def build_mysekai_help_ios(host: str) -> str:
    return f"""#!name=MySekai Upload Helper
#!desc=自动抓取 MySekai 数据并上传到 LunaBot 服务器 ({host})
#!author=Nene-LunaBot
#!system=ios
#!redirect=3
#!mitm=3
#!total=7
# 按需修改配置注释

[URL Rewrite]
# 日服 (取消注释以启用)
# ^https:\\/\\/production-game-api\\.sekai\\.colorfulpalette\\.org\\/api\\/user\\/(\\d+)\\/mysekai\\?isForceAllReloadOnlyMysekai=False$ https://production-game-api.sekai.colorfulpalette.org/api/user/$1/mysekai?isForceAllReloadOnlyMysekai=True 307
# ^https:\\/\\/submit\\.backtrace\\.io\\/ reject

# 国服
^https:\\/\\/mkcn-prod-public-60001-1\\.dailygn\\.com\\/api\\/user\\/(\\d+)\\/mysekai\\?isForceAllReloadOnlyMysekai=False$ https://mkcn-prod-public-60001-1.dailygn.com/api/user/$1/mysekai?isForceAllReloadOnlyMysekai=True 307
^https:\\/\\/mkcn-prod-public-60001-2\\.dailygn\\.com\\/api\\/user\\/(\\d+)\\/mysekai\\?isForceAllReloadOnlyMysekai=False$ https://mkcn-prod-public-60001-2.dailygn.com/api/user/$1/mysekai?isForceAllReloadOnlyMysekai=True 307
^https:\\/\\/submit\\.backtrace\\.io\\/ reject

[Script]
# 国服
SCRIPT_cn_mysekai_1 = type=http-response, requires-body=1, binary-body-mode=1, max-size=100000000, timeout=60, pattern=^https:\\/\\/mkcn-prod-public-60001-1\\.dailygn\\.com\\/api\\/user\\/(\\d+)\\/mysekai\\?isForceAllReloadOnlyMysekai=True, script-path={_build_upload_script_url(host, "cn", "mysekai")}
SCRIPT_cn_mysekai_2 = type=http-response, requires-body=1, binary-body-mode=1, max-size=100000000, timeout=60, pattern=^https:\\/\\/mkcn-prod-public-60001-2\\.dailygn\\.com\\/api\\/user\\/(\\d+)\\/mysekai\\?isForceAllReloadOnlyMysekai=True, script-path={_build_upload_script_url(host, "cn", "mysekai")}

# 日服 (取消注释以启用)
# SCRIPT_jp_mysekai = type=http-response, requires-body=1, binary-body-mode=1, max-size=100000000, timeout=60, pattern=^https:\\/\\/production-game-api\\.sekai\\.colorfulpalette\\.org\\/api\\/user\\/(\\d+)\\/mysekai\\?isForceAllReloadOnlyMysekai=True, script-path={_build_upload_script_url(host, "jp", "mysekai")}

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
使用后需要关闭进程必须使用停止脚本，初次下载之后都直接执行，下载命令见上，如果后续报错如网络错误或地址已经存在，请先确认是否按要求关闭进程
抓包时直接执行 catcher.sh 即可，然后运行游戏，所提取数据仅供Nenebot使用

## 注意事项
1. 关闭进程请使用下面的脚本，仅关闭终端无法关闭进程
2. 如果设置代理失败则需要手动设置代理，在设置中的wifi处长按当前wifi，设置为手动代理，主机名设置为127.0.0.1,端口设置为8888，修改完后重启虚拟机
"""


def build_ios_upload_script(host: str, region: str, data_type: str) -> str:
    upload_url = _build_ios_upload_api_url(host, region, data_type)
    return f"""const scriptName = "nene_upload_ios.js";
const version = "1.0.0";
const chunkSize = 1024 * 1024;
const uploadUrl = "{upload_url}";
const uploadId = `${{Date.now().toString(36)}}-${{Math.random().toString(36).slice(2, 10)}}`;

const isQuanX = typeof $task !== "undefined";
const isSurgeLike = typeof $httpClient !== "undefined";

function done(value) {{
    if (typeof $done === "function") {{
        $done(value || {{}});
    }}
}}

function log(message) {{
    console.log(`[${{scriptName}} v${{version}}] ${{message}}`);
}}

function postChunk(options, callback) {{
    if (isQuanX) {{
        $task.fetch({{
            url: options.url,
            method: "POST",
            headers: options.headers,
            body: options.body,
        }}).then(
            (response) => callback(null, response, response.body || ""),
            (error) => callback(error, null, ""),
        );
        return;
    }}
    if (isSurgeLike) {{
        $httpClient.post(options, (error, response, data) => callback(error, response, data || ""));
        return;
    }}
    callback(new Error("Unsupported runtime"), null, "");
}}

const body = typeof $response !== "undefined" ? $response.body : "";
const originalUrl = typeof $request !== "undefined" ? $request.url : "";

if (!body || !originalUrl) {{
    log("skip upload because response body or request url is missing");
    done({{}});
}} else {{
    const totalChunks = Math.ceil(body.length / chunkSize);
    let currentIndex = 0;

    function sendNext() {{
        if (currentIndex >= totalChunks) {{
            log(`upload completed with ${{totalChunks}} chunks`);
            done({{}});
            return;
        }}

        const start = currentIndex * chunkSize;
        const end = Math.min(start + chunkSize, body.length);
        const chunk = body.slice(start, end);
        const chunkNumber = currentIndex + 1;

        log(`uploading chunk ${{chunkNumber}}/${{totalChunks}}`);
        postChunk(
            {{
                url: uploadUrl,
                headers: {{
                    "Content-Type": "application/octet-stream",
                    "X-Script-Version": version,
                    "X-Original-Url": originalUrl,
                    "X-Upload-Id": uploadId,
                    "X-Chunk-Index": String(currentIndex),
                    "X-Total-Chunks": String(totalChunks),
                }},
                body: chunk,
            }},
            (error, response, responseBody) => {{
                const status = response ? (response.statusCode || response.status) : 0;
                if (error || status < 200 || status >= 300) {{
                    log(`chunk ${{chunkNumber}} failed: ${{error || status}}`);
                    if (responseBody) {{
                        log(`response body: ${{String(responseBody)}}`);
                    }}
                    done({{}});
                    return;
                }}
                currentIndex += 1;
                sendNext();
            }},
        );
    }}

    sendNext();
}}
"""
