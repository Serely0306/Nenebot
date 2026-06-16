from __future__ import annotations

from core.runtime import REGION_TO_KEYSET

try:
    import msgpack
    from sssekai.crypto.APIManager import decrypt, SEKAI_APIMANAGER_KEYSETS

    DECRYPT_AVAILABLE = True
except ImportError:
    DECRYPT_AVAILABLE = False
    print("警告: sssekai 或 msgpack 未安装，二进制文件解密功能不可用")
    print("请运行: pip install sssekai msgpack")


def decrypt_binary_data(binary_data: bytes, region: str) -> dict:
    if not DECRYPT_AVAILABLE:
        raise RuntimeError("解密功能不可用，请安装 sssekai 和 msgpack")

    keyset_name = REGION_TO_KEYSET.get(region)
    if keyset_name not in SEKAI_APIMANAGER_KEYSETS:
        raise ValueError(f"不支持的区服密钥: {region}")

    keyset = SEKAI_APIMANAGER_KEYSETS[keyset_name]
    try:
        decrypted_data = decrypt(binary_data, keyset)
    except Exception as exc:
        raise ValueError(f"解密失败: {exc}") from exc

    try:
        return msgpack.unpackb(decrypted_data, raw=False)
    except Exception as exc:
        raise ValueError(f"msgpack 反序列化失败: {exc}") from exc


def convert_to_serializable(obj):
    if isinstance(obj, dict):
        return {k: convert_to_serializable(v) for k, v in obj.items()}
    if isinstance(obj, list):
        return [convert_to_serializable(item) for item in obj]
    if isinstance(obj, bytes):
        try:
            return obj.decode("utf-8")
        except Exception:
            return obj.hex()
    return obj
