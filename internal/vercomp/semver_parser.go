package vercomp

import "github.com/Masterminds/semver/v3"

type SemVerParser struct{}

func (p *SemVerParser) Name() string {
	return "SemVerParser"
}

func (p *SemVerParser) CanParse(v string) bool {
	_, err := semver.StrictNewVersion(v)
	return err == nil
}

func (p *SemVerParser) Parse(v string) (interface{}, error) {
	return semver.StrictNewVersion(v)
}

func (p *SemVerParser) Compare(a, b interface{}) int {
	verA := a.(*semver.Version)
	verB := b.(*semver.Version)
	return verA.Compare(verB)
}
