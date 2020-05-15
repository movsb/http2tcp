package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

var config = Config{}

func main() {
	fp, err := os.Open(`http2tcp.yml`)
	if err != nil {
		panic(err)
	}
	defer fp.Close()
	if err := yaml.NewDecoder(fp).Decode(&config); err != nil {
		panic(err)
	}
	switch os.Args[1] {
	case `s`:
		server()
	case `c`:
		client()
	}
}
