package model

type CreateResourceRequest struct {
	ID          string `json:"id" validate:"required,min=3,max=64,slug"`
	Name        string `json:"name" validate:"required"`
	Description string `json:"description" validate:"max=255"`
	UpdateType  string `json:"update_type" validate:"omitempty,oneof=full incremental"`
}

type ListResourceRequest struct {
	Sort   string `query:"sort"`
	Offset int    `query:"offset" validate:"min=0"`
	Limit  int    `query:"limit" validate:"omitempty,min=1,max=100"`
}

type CreateVersionRequest struct {
	Name     string `json:"name" form:"name" validate:"required"`
	OS       string `json:"os" form:"os"`
	Arch     string `json:"arch" form:"arch"`
	Channel  string `json:"channel" form:"channel"`
	Filename string `json:"filename" form:"filename" validate:"required"`
}

type ListVersionRequest struct {
	Sort   string `query:"sort"`
	Offset int    `query:"offset" validate:"min=0"`
	Limit  int    `query:"limit" validate:"omitempty,min=1,max=100"`
}

type CreateVersionCallBackRequest struct {
	Name    string `json:"name" form:"name" validate:"required"`
	OS      string `json:"os" form:"os"`
	Arch    string `json:"arch" form:"arch"`
	Channel string `json:"channel" form:"channel"`
	Key     string `json:"key" form:"key" validate:"required"`
}

type GetLatestVersionRequest struct {
	ResourceID     string
	CurrentVersion string `query:"current_version"`
	OS             string `query:"os"`
	Arch           string `query:"arch"`
	Channel        string `query:"channel"`
	CDK            string `query:"cdk"`
	UserAgent      string `query:"user_agent"`
}

type UpdateReleaseNoteRequest struct {
	VersionName string `json:"version_name"`
	Channel     string `json:"channel"`
	Content     string `json:"content"`
}

type UpdateCustomDataRequest struct {
	VersionName string `json:"version_name"`
	Channel     string `json:"channel"`
	Content     string `json:"content"`
}
