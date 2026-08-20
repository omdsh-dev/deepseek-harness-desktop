package main

import (
	"os"

	"github.com/omdsh-dev/dsh-web-desktopify/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
