package model

import "time"

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
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	// UpdateType is the type of the update, it can be "full" or "incremental"
	UpdateType     string `json:"update_type,omitempty"`
	CustomData     string `json:"custom_data,omitempty"`
	ReleaseNote    string `json:"release_note"`
	Filesize       int64  `json:"filesize,omitempty"`
	CDKExpiredTime int64  `json:"cdk_expired_time,omitempty"`
}

type GetVersionStatusResponseData struct {
	Status int `json:"status"`
}

type CreateVersionCallBackResponseData struct {
	StatusKey string `json:"status_key"`
}

// PageData is the generic paginated envelope for admin list endpoints.
type PageData struct {
	List     any `json:"list"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// ResourceItem is a resource row in the admin resource list.
type ResourceItem struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	UpdateType  string    `json:"update_type"`
	CreatedAt   time.Time `json:"created_at"`
}

// ResourceDetailData is the admin resource detail payload.
type ResourceDetailData struct {
	ResourceItem
	VersionCount int `json:"version_count"`
}

// VersionItem is a version row in the admin version list.
type VersionItem struct {
	ID        int       `json:"id"`
	Channel   string    `json:"channel"`
	Name      string    `json:"name"`
	Number    uint64    `json:"number"`
	CreatedAt time.Time `json:"created_at"`
}
