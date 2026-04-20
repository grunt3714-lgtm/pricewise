package main

import (
	"os"

	"github.com/grunt3714-lgtm/pricewise/internal/cli"
	"github.com/grunt3714-lgtm/pricewise/store"
	"github.com/grunt3714-lgtm/pricewise/store/flipp"
)

func main() {
	os.Exit(cli.RunSingle("food4less", func(zip string) store.Store {
		return flipp.New("food4less", "Food 4 Less", zip)
	}, os.Args[1:]))
}
