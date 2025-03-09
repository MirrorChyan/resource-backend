package model

type CreateResourceRequest struct {
	ID          string `json:"id" validate:"required,min=3,max=64,slug"`
	Name        string `json:"name" validate:"required"`
	Description string `json:"description" validate:"max=255"`
	UpdateType  string `json:"update_type" validate:"omitempty,oneof=full incremental"`
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
