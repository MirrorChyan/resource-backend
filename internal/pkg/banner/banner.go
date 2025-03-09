package banner

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed banner.txt
var banner string

func init() {
	fmt.Println(strings.TrimSpace(banner))
}
