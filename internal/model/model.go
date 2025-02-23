package model

type ValidateCDKRequest struct {
	CDK      string `json:"cdk"`
	Resource string `json:"resource"`
	UA       string `json:"ua"`
	IP       string `json:"ip"`
}

type ValidateCDKResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type ValidateUploaderResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type UpdateInfo struct {
	RelPath    string
	SHA256     string
	UpdateType string
}

type DistributeInfo struct {
	Region   string `json:"region"`
	CDK      string `json:"cdk"`
	RelPath  string `json:"rel_path"`
	Resource string `json:"resource"`
}
