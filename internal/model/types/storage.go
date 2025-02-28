package types

type Update string

const (
	UpdateFull        Update = "full"
	UpdateIncremental Update = "incremental"
)

func (u Update) String() string {
	return string(u)
}
