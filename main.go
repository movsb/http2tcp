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
	runAsServer := flag.BoolP(`server`, `s`, false, `Run as server.`)
	runAsClient := flag.BoolP(`client`, `c`, false, `Run as client.`)

	listenAddr := flag.StringP(`listen`, `l`, ``, `Listen address (client & server)`)
	serverEndpoint := flag.StringP(`endpoint`, `e`, ``, `Server endpoint.`)
	destination := flag.StringP(`destination`, `d`, ``, `The destination address to connect to`)

	token := flag.StringP(`token`, `T`, ``, `The token used between client and server`)

	help := flag.BoolP(`help`, `h`, false, `Show this help`)

	flag.Parse()

	if !*help && !*runAsServer && !*runAsClient {
		log.Fatalln(`either -s or -c must be specified`)
	}
	if *help {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
		os.Exit(1)
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
