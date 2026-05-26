package main

import (
	"fmt"
	"io"
)

var (
	version = "dev"
	commit  = "unknown"
	built   = "unknown"
)

func printVersion(w io.Writer) {
	fmt.Fprintf(w, "bkt version %s\ncommit %s\nbuilt %s\n", version, commit, built)
}
