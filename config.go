package main

// Path ...
type Path struct {
	Name   string `yaml:"name"`
	Token  string `yaml:"token"`
	Local  string `yaml:"local"`
	Remote string `yaml:"remote"`
}

// Config ...
type Config struct {
	Prefix string `yaml:"prefix"`
	Token  string `yaml:"token"`
	Paths  []Path `yaml:"paths"`
}
