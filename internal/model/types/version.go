package types

type Channel string

const (
	ChannelStable Channel = "stable"
	ChannelBeta   Channel = "beta"
	ChannelAlpha  Channel = "alpha"
)

func (c Channel) String() string {
	return string(c)
}
