package model

import "time"

type CreateResourceResponseData struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ResourceResponseItem struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type ListResourceResponseData struct {
	List    []*ResourceResponseItem `json:"list"`
	Offset  int                     `json:"offset"`
	Limit   int                     `json:"limit"`
	Total   int                     `json:"total"`
	HasMore bool                    `json:"has_more"`
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
	UpdateType     string    `json:"update_type,omitempty"`
	CustomData     string    `json:"custom_data,omitempty"`
	ReleaseNote    string    `json:"release_note"`
	Filesize       int64     `json:"filesize,omitempty"`
	CDKExpiredTime int64     `json:"cdk_expired_time,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}
