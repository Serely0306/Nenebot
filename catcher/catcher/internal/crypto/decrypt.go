package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

// RegionKeySet 各区服的加密密钥
type RegionKeySet struct {
	Key []byte
	IV  []byte
}

// 各区服的 AES 密钥和 IV
// 来源: haruki-sekai-configs.yaml
// Key hex: 6732666343305a637a4e394d544a3631
// IV hex:  6d737833495630693958453575595a31
var RegionKeys = map[string]RegionKeySet{
	"jp": {
		Key: []byte{0x67, 0x32, 0x66, 0x63, 0x43, 0x30, 0x5a, 0x63, 0x7a, 0x4e, 0x39, 0x4d, 0x54, 0x4a, 0x36, 0x31},
		IV:  []byte{0x6d, 0x73, 0x78, 0x33, 0x49, 0x56, 0x30, 0x69, 0x39, 0x58, 0x45, 0x35, 0x75, 0x59, 0x5a, 0x31},
	},
	"en": {
		Key: []byte{0x67, 0x32, 0x66, 0x63, 0x43, 0x30, 0x5a, 0x63, 0x7a, 0x4e, 0x39, 0x4d, 0x54, 0x4a, 0x36, 0x31},
		IV:  []byte{0x6d, 0x73, 0x78, 0x33, 0x49, 0x56, 0x30, 0x69, 0x39, 0x58, 0x45, 0x35, 0x75, 0x59, 0x5a, 0x31},
	},
	"tw": {
		Key: []byte{0x67, 0x32, 0x66, 0x63, 0x43, 0x30, 0x5a, 0x63, 0x7a, 0x4e, 0x39, 0x4d, 0x54, 0x4a, 0x36, 0x31},
		IV:  []byte{0x6d, 0x73, 0x78, 0x33, 0x49, 0x56, 0x30, 0x69, 0x39, 0x58, 0x45, 0x35, 0x75, 0x59, 0x5a, 0x31},
	},
	"kr": {
		Key: []byte{0x67, 0x32, 0x66, 0x63, 0x43, 0x30, 0x5a, 0x63, 0x7a, 0x4e, 0x39, 0x4d, 0x54, 0x4a, 0x36, 0x31},
		IV:  []byte{0x6d, 0x73, 0x78, 0x33, 0x49, 0x56, 0x30, 0x69, 0x39, 0x58, 0x45, 0x35, 0x75, 0x59, 0x5a, 0x31},
	},
	"cn": {
		Key: []byte{0x67, 0x32, 0x66, 0x63, 0x43, 0x30, 0x5a, 0x63, 0x7a, 0x4e, 0x39, 0x4d, 0x54, 0x4a, 0x36, 0x31},
		IV:  []byte{0x6d, 0x73, 0x78, 0x33, 0x49, 0x56, 0x30, 0x69, 0x39, 0x58, 0x45, 0x35, 0x75, 0x59, 0x5a, 0x31},
	},
}

// PKCS7Unpad 移除 PKCS7 填充
func PKCS7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	padding := int(data[len(data)-1])
	if padding > len(data) || padding == 0 {
		return nil, fmt.Errorf("invalid padding")
	}
	for i := len(data) - padding; i < len(data); i++ {
		if data[i] != byte(padding) {
			return nil, fmt.Errorf("invalid padding")
		}
	}
	return data[:len(data)-padding], nil
}

// Decrypt 解密游戏数据
func Decrypt(data []byte, region string) ([]byte, error) {
	keySet, ok := RegionKeys[region]
	if !ok {
		return nil, fmt.Errorf("unsupported region: %s", region)
	}

	block, err := aes.NewCipher(keySet.Key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	if len(data) < aes.BlockSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	if len(data)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext is not a multiple of the block size")
	}

	mode := cipher.NewCBCDecrypter(block, keySet.IV)

	decrypted := make([]byte, len(data))
	mode.CryptBlocks(decrypted, data)

	// 移除 PKCS7 填充
	decrypted, err = PKCS7Unpad(decrypted)
	if err != nil {
		return nil, fmt.Errorf("unpad: %w", err)
	}

	return decrypted, nil
}

// DecryptAndUnpack 解密并反序列化 msgpack 数据
func DecryptAndUnpack(data []byte, region string) (map[string]interface{}, error) {
	decrypted, err := Decrypt(data, region)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	decoder := msgpack.NewDecoder(bytes.NewReader(decrypted))
	decoder.SetMapDecoder(func(dec *msgpack.Decoder) (interface{}, error) {
		return dec.DecodeMap()
	})

	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("msgpack decode: %w", err)
	}

	return result, nil
}
