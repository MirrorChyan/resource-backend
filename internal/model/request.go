package model

type CreateResourceRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type GetLatestVersionRequest struct {
	CurrentVersion string `query:"current_version"`
	OS             string `query:"os"`
	Arch           string `query:"arch"`
	Channel        string `query:"channel"`
	CDK            string `query:"cdk"`
	SpID           string `query:"sp_id"`
	UserAgent      string `query:"user_agent"`
}

type UpdateReleaseNoteDetailRequest struct {
	VersionName string `json:"version_name"`
	Content     string `json:"content"`
}

type UpdateReleaseNoteSummaryRequest struct {
	VersionName string `json:"version_name"`
	Content     string `json:"content"`
}
