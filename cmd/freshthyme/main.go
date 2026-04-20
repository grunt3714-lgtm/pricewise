package main

import (
	"os"

	"github.com/grunt3714-lgtm/pricewise/internal/cli"
	"github.com/grunt3714-lgtm/pricewise/store"
	"github.com/grunt3714-lgtm/pricewise/store/flipp"
)

func main() {
	os.Exit(cli.RunSingle("freshthyme", func(zip string) store.Store {
		return flipp.New("freshthyme", "Fresh Thyme Market", zip)
	}, os.Args[1:]))
}
