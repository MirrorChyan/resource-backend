package vercomp

import (
	"time"

	"go.uber.org/zap"
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
	Name() string
	CanParse(version string) bool
	Parse(version string) (interface{}, error)
	Compare(a, b interface{}) int
}

type VersionComparator struct {
	parsers []Parser
}

func NewDefaultParsers() []Parser {
	return []Parser{
		&SemVerParser{},
		&DateTimeParser{
			Layouts: []string{
				time.RFC3339,
				time.DateTime,
				"2006-01-02 15:04:05.000",
				"20060102150405",
			},
		},
	}
}

func NewComparator(parsers ...Parser) *VersionComparator {
	var usedParsers []Parser

	if len(parsers) > 0 {
		usedParsers = parsers
	} else {
		usedParsers = NewDefaultParsers()
	}

	return &VersionComparator{
		parsers: usedParsers,
	}
}

func (c *VersionComparator) AddParser(p Parser) {
	c.parsers = append(c.parsers, p)
}

func (c *VersionComparator) Compare(v1, v2 string) CompareResult {
	// try parsing both versions
	parsed1, parser1 := c.parseVersion(v1)
	parsed2, parser2 := c.parseVersion(v2)

	// must use the same type of parser
	if parser1 != nil && parser1 == parser2 {
		return CompareResult{
			Comparable: true,
			Result:     parser1.Compare(parsed1, parsed2),
		}
	}
	return CompareResult{Comparable: false}
}

func (c *VersionComparator) parseVersion(v string) (interface{}, Parser) {
	parser, ok := c.canParseWithAnyParser(v)
	if !ok {
		return nil, nil

	}

	parsed, err := parser.Parse(v)
	if err != nil {
		zap.L().Error("Failed to parse version",
			zap.String("version name", v),
			zap.String("parser name", parser.Name()),
			zap.Error(err),
		)
		return nil, nil
	}

	return parsed, parser
}

func (c *VersionComparator) IsVersionParsable(version string) bool {
	_, ok := c.canParseWithAnyParser(version)
	return ok
}

func (c *VersionComparator) canParseWithAnyParser(version string) (Parser, bool) {
	for _, p := range c.parsers {
		if p.CanParse(version) {
			return p, true
		}
	}
	return nil, false
}
