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

type UpdateVersionReleaseNoteRequest struct {
	VersionName        string `json:"version_name"`
	ReleaseNoteSummary string `json:"release_note_summary"`
	ReleaseNoteDetail  string `json:"release_note_detail"`
}
