package main

import (
	"os"
)

var config = Config{
	Prefix: `nginx2tcp`,
	Token:  `token`,
	Paths: []Path{
		{
			Name:   `test`,
			Token:  `test`,
			Local:  `localhost:3333`,
			Remote: `localhost:22`,
		},
	},
}

func main() {
	switch os.Args[1] {
	case `s`:
		server()
	case `c`:
		client()
	}
}
