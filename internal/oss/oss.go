package oss

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"strings"
	"time"

	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/bytedance/sonic"
)

func AcquirePolicyToken(prefix, file string) (*SignaturePolicyToken, error) {
	var (
		oss       = config.GConfig.OSS
		accessKey = oss.AccessKey
		secretKey = oss.SecretKey
		host      = oss.Endpoint

		now      = time.Now().Unix()
		expireAt = now + expireTime
		cfg      ConfigStruct
	)

	cfg.Expiration = time.Unix(expireAt, 0).UTC().Format("2006-01-02T15:04:05Z")
	cfg.Conditions = append(cfg.Conditions, array{
		"starts-with", "$key", prefix,
	})

	cfg.Conditions = append(cfg.Conditions, array{
		"content-length-range", 0, bodyLimit,
	})

	result, err := sonic.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	policy := base64.StdEncoding.EncodeToString(result)

	h := hmac.New(sha1.New, []byte(secretKey))
	h.Write([]byte(policy))
	signature := base64.StdEncoding.EncodeToString(h.Sum(nil))

	return &SignaturePolicyToken{
		AccessKeyId: accessKey,
		Host:        host,
		Signature:   signature,
		Policy:      policy,
		Name:        file,
		Key:         strings.Join([]string{prefix, file}, "/"),
	}, nil
}
