package exportdomain

import (
	"errors"
	"net/url"
	"strings"
)

var storageSchemeValues = []string{"oss", "s3", "cos", "obs"}

// ValidateControlledStorageURI 校验对象存储输出 URI 的 scheme、Bucket、路径和查询参数。
// 密钥参数不属于 URI 契约，必须由执行槽位提供；未知 scheme 或查询参数一律失败关闭。
func ValidateControlledStorageURI(outputKind, uri string) error {
	if uri == "" || len(uri) > 4096 || strings.ContainsRune(uri, 0) || strings.ContainsAny(uri, "\r\n") {
		return errors.New("v6 storage uri is invalid")
	}
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || !strings.HasPrefix(parsed.Path, "/") {
		return errors.New("v6 storage uri must be scheme://bucket/path")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if !contains(storageSchemeValues, scheme) || !strings.EqualFold(scheme, outputKind) {
		return errors.New("v6 storage uri scheme is unsupported")
	}
	for key := range parsed.Query() {
		if key != "endpoint" && key != "region" && key != "storage-class" {
			return errors.New("v6 storage uri has unsupported parameters")
		}
	}
	return nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
