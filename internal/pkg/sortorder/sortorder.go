package sortorder

type Order string

const (
	Newest  Order = "newest"
	Oldest  Order = "oldest"
	Default       = Newest
)

func Parse(str string) Order {
	switch str {
	case string(Newest):
		return Newest
	case string(Oldest):
		return Oldest
	default:
		return Default
	}
}
