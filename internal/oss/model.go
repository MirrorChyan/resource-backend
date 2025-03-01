package oss

const (
	expireTime = int64(3600)
	bodyLimit  = 1000 * 1024 * 1024
)

type ConfigStruct struct {
	Expiration string  `json:"expiration"`
	Conditions [][]any `json:"conditions"`
}

type SignaturePolicyToken struct {
	AccessKeyId string `json:"access_key"`
	Host        string `json:"host"`
	Signature   string `json:"signature"`
	Policy      string `json:"policy"`
	Key         string `json:"key"`
	Name        string `json:"name"`
}

type array []any
