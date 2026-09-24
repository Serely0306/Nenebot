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
2. 如果设置代理失败则需要手动设置代理，在设置中的wifi处长按当前wifi，设置为手动代理，主机名设置为127.0.0.1,端口设置为8888，修改完后重启游戏
3. catcher 配置中的 upload_server 应填写服务根地址，不要附加 /upload 前缀
"""


def build_ios_upload_script(host: str, region: str, data_type: str) -> str:
    upload_url = _build_ios_upload_api_url(host, region, data_type)
    return f"""const uploadUrl = "{upload_url}";
const chunkSize = 1024 * 1024;
// Leave time for the original game response before the module's 60s timeout.
const deadline = Date.now() + 50000;
let finished = false;
const timer = setTimeout(() => {{
  console.log("Upload timed out; passing through the game response");
  finish();
}}, 50000);

function finish() {{
  if (finished) return;
  finished = true;
  clearTimeout(timer);
  $done({{}});
}}

function uploadChunk(chunk, uploadId, chunkIndex, totalChunks, originalUrl) {{
  if (finished || Date.now() >= deadline) return Promise.reject(new Error("Upload timed out"));
  return new Promise((resolve, reject) => $httpClient.post({{
    url: uploadUrl,
    timeout: Math.max(1, Math.ceil((deadline - Date.now()) / 1000)),
    headers: {{
      "Content-Type": "application/octet-stream",
      "X-Upload-Id": uploadId,
      "X-Original-Url": originalUrl,
      "X-Chunk-Index": String(chunkIndex),
      "X-Total-Chunks": String(totalChunks),
    }},
    body: chunk,
  }}, (error, response, body) => {{
    if (error) {{
      reject(new Error(String(error)));
    }} else if (!response || response.status < 200 || response.status >= 300) {{
      console.log("response body:", body || "");
      reject(new Error("Upload failed: HTTP " + (response ? response.status : "unknown")));
    }} else {{
      try {{
        const result = JSON.parse(body);
        if (result.success !== true || result.complete !== (chunkIndex === totalChunks - 1)) {{
          throw new Error("Server did not acknowledge the uploaded chunk");
        }}
        resolve(result);
      }} catch (error) {{
        reject(error);
      }}
    }}
  }}));
}}

async function uploadResponse() {{
  if (typeof $response === "undefined" || Number($response.status) !== 200) {{
    console.log("Upload skipped: game response was not successful");
    return;
  }}
  const body = $response.body;
  if (!body || body.byteLength === 0) {{
    console.log("Upload skipped: response body is empty");
    return;
  }}

  let binaryBody;
  if (body instanceof ArrayBuffer) {{
    binaryBody = new Uint8Array(body);
  }} else if (ArrayBuffer.isView(body)) {{
    binaryBody = new Uint8Array(body.buffer, body.byteOffset, body.byteLength);
  }} else {{
    throw new Error("Expected binary response; enable binary-body-mode");
  }}
  const totalChunks = Math.ceil(binaryBody.byteLength / chunkSize);
  const uploadId = "ios-" + Date.now() + "-" + Math.random().toString(36).slice(2);
  const originalUrl = typeof $request !== "undefined" ? $request.url : "";
  if (!originalUrl) throw new Error("Missing original request URL");
  for (let index = 0; index < totalChunks; index++) {{
    const start = index * chunkSize;
    const end = Math.min(start + chunkSize, binaryBody.byteLength);
    await uploadChunk(binaryBody.slice(start, end), uploadId, index, totalChunks, originalUrl);
  }}
  console.log("Upload complete: " + binaryBody.byteLength + " bytes in " + totalChunks + " chunk(s)");
}}

uploadResponse()
  .catch(error => console.log("Upload failed:", error.message || error))
  .finally(finish);
"""
