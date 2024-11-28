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
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	runAsServer := flag.BoolP(`server`, `s`, false, `Run as server. [S]`)
	runAsClient := flag.BoolP(`client`, `c`, false, `Run as client. [C]`)

	serverEndpoint := flag.StringP(`endpoint`, `e`, ``, `Server endpoint. [C]`)
	token := flag.StringP(`token`, `t`, ``, `The token used between client and server [SC]`)
	userAgent := flag.String(`user-agent`, ``, `Use this User-Agent instead of the default Go-http-client/1.1 [C]`)

	listenAddr := flag.StringP(`listen`, `l`, ``, `Listen address [SC]`)
	destination := flag.StringP(`destination`, `d`, ``, `The destination address to connect to [C]`)

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
		return
	}
	if *runAsServer {
		s := &Server{
			Token: *token,
		}
		if err := http.ListenAndServe(*listenAddr, s); err != nil {
			log.Fatalln(err)
		}
		return
	}
	if *runAsClient {
		c := &Client{
			Server: *serverEndpoint,
			Token:  *token,
		}
		c.SetUserAgent(*userAgent)
		if *listenAddr != `` {
			c.Serve(*listenAddr, *destination)
		} else {
			c.Std(*destination)
		}
		return
	}
}
