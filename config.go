package main

import (
	"io/ioutil"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Tasks map[string]Task
}

type Task struct {
	Name   string
	Cmd    string
	Nowait bool
}

func parseConfig() (config Config, err error) {
	bts, err := ioutil.ReadFile("onchange.yml")
	if err != nil {
		return
	}
	err = yaml.Unmarshal(bts, &config)
	return
}
