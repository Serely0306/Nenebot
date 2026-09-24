from __future__ import annotations

from core.app_config import load_upload_config

try:
    import msgpack
    from Crypto.Cipher import AES

    DECRYPT_AVAILABLE = True
except ImportError:
    DECRYPT_AVAILABLE = False
    print("警告: msgpack 或 pycryptodome 未安装，二进制文件解密功能不可用")
    print("请运行: pip install msgpack pycryptodome")


def _load_keyset(region: str) -> tuple[bytes, bytes]:
    keyset = load_upload_config().crypto.keysets.get(region)
    if keyset is None:
        raise ValueError(f"未配置区服密钥: {region}")
    try:
        key = bytes.fromhex(keyset.key_hex)
        iv = bytes.fromhex(keyset.iv_hex)
    except ValueError as exc:
        raise ValueError(f"区服密钥配置无效: {region}") from exc
    if len(key) not in (16, 24, 32) or len(iv) != AES.block_size:
        raise ValueError(f"区服密钥长度无效: {region}")
    return key, iv


def _pkcs7_unpad(data: bytes) -> bytes:
    if not data:
        raise ValueError("解密结果为空")
    padding = data[-1]
    if padding < 1 or padding > AES.block_size or padding > len(data):
        raise ValueError("PKCS7 padding 无效")
    if data[-padding:] != bytes([padding]) * padding:
        raise ValueError("PKCS7 padding 无效")
    return data[:-padding]


def decrypt_binary_data(binary_data: bytes, region: str) -> dict:
    if not DECRYPT_AVAILABLE:
        raise RuntimeError("解密功能不可用，请安装 msgpack 和 pycryptodome")

    try:
        key, iv = _load_keyset(region)
        if len(binary_data) < AES.block_size or len(binary_data) % AES.block_size:
            raise ValueError("密文长度不是 AES 块大小的整数倍")
        decrypted_data = _pkcs7_unpad(
            AES.new(key, AES.MODE_CBC, iv).decrypt(binary_data)
        )
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
