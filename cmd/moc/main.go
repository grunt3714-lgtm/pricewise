package main

import (
	"os"

	"github.com/grunt3714-lgtm/pricewise/internal/cli"
	"github.com/grunt3714-lgtm/pricewise/store"
	"github.com/grunt3714-lgtm/pricewise/store/moc"
)

func main() {
	os.Exit(cli.RunSingle("moc", func(_ string) store.Store {
		return moc.New()
	}, os.Args[1:]))
}
