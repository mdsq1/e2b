package e2b

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

// getSignature 生成用于文件操作的签名 URL。
// 使用 SHA256 哈希和 Base64 编码，基于路径、操作类型、用户名、访问令牌和可选的过期时间生成签名。
func getSignature(path, operation, user, accessToken string, expirationSec *int) (sig string, exp *int64, err error) {
	if accessToken == "" {
		return "", nil, fmt.Errorf("access token is not set and signature cannot be generated")
	}

	var expiration *int64
	if expirationSec != nil {
		v := time.Now().Unix() + int64(*expirationSec)
		expiration = &v
	}

	raw := fmt.Sprintf("%s:%s:%s:%s", path, operation, user, accessToken)
	if expiration != nil {
		raw = fmt.Sprintf("%s:%d", raw, *expiration)
	}

	hash := sha256.Sum256([]byte(raw))
	encoded := base64.StdEncoding.EncodeToString(hash[:])
	encoded = strings.TrimRight(encoded, "=")

	return fmt.Sprintf("v1_%s", encoded), expiration, nil
}
