//go:build ignore

package main

import (
	"log"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

// The ent CLI is not used: built with this module's dependencies its table printer fails to
// compile, built with its own it loads packages through an x/tools too old for this Go.
func main() {
	if err := entc.Generate("./schema", &gen.Config{}); err != nil {
		log.Fatalf("running ent codegen: %v", err)
	}
}
