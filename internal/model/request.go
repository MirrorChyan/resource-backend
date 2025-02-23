package model

type CreateResourceRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
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
