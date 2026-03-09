"""
MySekai 数据上传服务

用于接收用户上传的 MySekai/Suite 数据，并提供查询、代理上传、MSR 渲染等接口。
运行方式:
    python server.py [--host HOST] [--port PORT]
"""

from __future__ import annotations

import argparse

from core.app_factory import create_app
from core.runtime import PROFILE_DB_PATH
from services.crypto import DECRYPT_AVAILABLE


app = create_app()


def main():
    parser = argparse.ArgumentParser(description="MySekai 数据上传服务")
    parser.add_argument("--host", default="0.0.0.0", help="监听地址 (默认: 0.0.0.0)")
    parser.add_argument("--port", type=int, default=5000, help="监听端口 (默认: 5000)")
    parser.add_argument("--debug", action="store_true", help="开启调试模式")
    args = parser.parse_args()

    decrypt_status = "已启用" if DECRYPT_AVAILABLE else "未安装依赖"
    db_status = "已连接" if PROFILE_DB_PATH.exists() else "未找到"

    print(
        f"""
============================================
 MySekai 数据上传服务 v2.0 - LunaBot
============================================
 本机访问: http://127.0.0.1:{args.port}
 局域网访问: http://<本机IP>:{args.port}
 二进制解密: {decrypt_status}
 绑定数据库: {db_status}
============================================
"""
    )

    app.run(host=args.host, port=args.port, debug=args.debug)


if __name__ == "__main__":
    main()
