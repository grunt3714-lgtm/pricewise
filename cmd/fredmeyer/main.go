package main

import (
	"os"

	"github.com/grunt3714-lgtm/pricewise/internal/cli"
	"github.com/grunt3714-lgtm/pricewise/store"
	"github.com/grunt3714-lgtm/pricewise/store/flipp"
	"github.com/grunt3714-lgtm/pricewise/store/kroger"
)

// Fred Meyer is multi-backend: prefer Kroger direct (full catalog, opt-in
// via KROGER_CLIENT_ID/SECRET) and fall back to Flipp (weekly ad only).
func main() {
	os.Exit(cli.RunSingle("fredmeyer", func(zip string) store.Store {
		return store.NewMulti("fredmeyer",
			kroger.New(zip),
			flipp.New("fredmeyer", "Fred Meyer", zip))
	}, os.Args[1:]))
}
