package main

import (
	"os"

	"github.com/grunt3714-lgtm/pricewise/internal/cli"
	"github.com/grunt3714-lgtm/pricewise/store"
	"github.com/grunt3714-lgtm/pricewise/store/capella"
)

func main() {
	os.Exit(cli.RunSingle("capella", func(_ string) store.Store {
		return capella.New()
	}, os.Args[1:]))
}
