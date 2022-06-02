package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	flag "github.com/spf13/pflag"
)

func main() {
	runAsServer := flag.BoolP(`server`, `s`, false, `Run as server. [S]`)
	runAsClient := flag.BoolP(`client`, `c`, false, `Run as client. [C]`)

	listenAddr := flag.StringP(`listen`, `l`, ``, `Listen address [SC]`)
	serverEndpoint := flag.StringP(`endpoint`, `e`, ``, `Server endpoint. [C]`)
	destination := flag.StringP(`destination`, `d`, ``, `The destination address to connect to [C]`)

	token := flag.StringP(`token`, `t`, ``, `The token used between client and server [SC]`)

	help := flag.BoolP(`help`, `h`, false, `Show this help`)

	flag.CommandLine.SortFlags = false
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "[S]: server side flag.\n[C]: client side flag.")
		os.Exit(1)
	}
	flag.Parse()

	if !*runAsServer && !*runAsClient || *help {
		flag.Usage()
	}
	if *runAsServer {
		s := NewServer(*token)
		if err := http.ListenAndServe(*listenAddr, s); err != nil {
			log.Fatalln(err)
		}
		return
	}
	if *runAsClient {
		c := NewClient(*serverEndpoint, *token)
		if *listenAddr != `` {
			c.Serve(*listenAddr, *destination)
		} else {
			c.Std(*destination)
		}
		return
	}
}
