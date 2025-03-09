package vercomp

import (
	"fmt"
	"time"
)

type DateTimeParser struct {
	Layouts []string // supported date formats
}

func (p *DateTimeParser) Name() string {
	return "DateTimeParser"
}

func (p *DateTimeParser) CanParse(v string) bool {
	for _, layout := range p.Layouts {
		if _, err := time.Parse(layout, v); err == nil {
			return true
		}
	}
	return false
}

func (p *DateTimeParser) Parse(v string) (interface{}, error) {
	for _, layout := range p.Layouts {
		if t, err := time.Parse(layout, v); err == nil {
			return t, nil
		}
	}
	return nil, fmt.Errorf("unsupported datetime format")
}

func (p *DateTimeParser) Compare(a, b interface{}) int {
	timeA := a.(time.Time)
	timeB := b.(time.Time)
	if timeA.Before(timeB) {
		return Less
	} else if timeA.After(timeB) {
		return Greater
	}
	return Equal
}
