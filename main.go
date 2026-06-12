package main

import (
	"os"

	"github.com/kirkzwy/captainbi-cli/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
