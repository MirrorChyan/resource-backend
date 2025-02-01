package vercomp

import (
	"time"

	"github.com/Masterminds/semver/v3"
)

// compare result
const (
	Less    = -1
	Equal   = 0
	Greater = 1
)

type CompareResult struct {
	Comparable bool
	Result     int // -1, 0, 1 (only when comparable)
}

type Parser interface {
	CanParse(version string) bool
	Parse(version string) (interface{}, error)
	Compare(a, b interface{}) int
}

type VersionComparator struct {
	parsers []Parser
}

func NewComparator() *VersionComparator {
	return &VersionComparator{
		parsers: []Parser{
			&SemVerParser{}, // 1: SemVer
			&DateTimeParser{ // 2: DataTime
				Layouts: []string{
					time.RFC3339,
					time.DateTime,
				},
			},
		},
	}
}

func (c *VersionComparator) AddParser(p Parser) {
	c.parsers = append(c.parsers, p)
}

func (c *VersionComparator) Compare(v1, v2 string) CompareResult {
	// try parsing both versions
	parsed1, parser := c.parseVersion(v1)
	parsed2, _ := c.parseVersion(v2)

	// must use the same type of parser
	if parser != nil && parser == c.getParserForValue(parsed2) {
		return CompareResult{
			Comparable: true,
			Result:     parser.Compare(parsed1, parsed2),
		}
	}
	return CompareResult{Comparable: false}
}

func (c *VersionComparator) parseVersion(v string) (interface{}, Parser) {
	for _, p := range c.parsers {
		if p.CanParse(v) {
			if parsed, err := p.Parse(v); err == nil {
				return parsed, p
			}
		}
	}
	return nil, nil
}

func (c *VersionComparator) getParserForValue(val interface{}) Parser {
	for _, p := range c.parsers {
		switch val.(type) {
		case *semver.Version:
			if _, ok := p.(*SemVerParser); ok {
				return p
			}
		case time.Time:
			if _, ok := p.(*DateTimeParser); ok {
				return p
			}
		}
	}
	return nil
}
