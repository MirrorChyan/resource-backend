package model

type CreateResourceResponseData struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type CreateVersionResponseData struct {
	Name   string `json:"name"`
	Number uint64 `json:"number"`
	OS     string `json:"os,omitempty"`
	Arch   string `json:"arch,omitempty"`
}

type QueryLatestResponseData struct {
	VersionName   string `json:"version_name"`
	VersionNumber uint64 `json:"version_number"`
	Url           string `json:"url,omitempty"`
	SHA256        string `json:"sha256,omitempty"`
	Channel       string `json:"channel"`
	OS            string `json:"os,omitempty"`
	Arch          string `json:"arch,omitempty"`
	// UpdateType is the type of the update, it can be "full" or "incremental"
	UpdateType  string `json:"update_type,omitempty"`
	CustomData string `json:"custom_data"`
	ReleaseNote  string `json:"release_note"`
}
