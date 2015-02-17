package main

import (
	"bufio"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"

	"github.com/fatih/color"
	"golang.org/x/exp/inotify"
)

var keyExtensions = []string{".go", ".tmpl", ".html", ".js"}

var cyan = color.New(color.FgCyan).SprintFunc()
var boldRed = color.New(color.FgRed).Add(color.Bold).SprintFunc()

func triggerRebuild(filename string) bool {
	// Ignore hidden files
	if filepath.Base(filename)[:1] == "." {
		return false
	}
	ext := filepath.Ext(filename)
	for _, extension := range keyExtensions {
		if ext == extension {
			return true
		}
	}
	return false
}

func processEvent(event *inotify.Event) {

	if !triggerRebuild(event.Name) {
		return
	}

	switch event.Mask {

	case inotify.IN_MODIFY:
		log.Printf(cyan("Modified: %s"), event.Name)

		rebuild()
	}
}

func rebuild() {
	cmd := exec.Command("go", "build")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			log.Println("reading standard input:", err)
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			fmt.Println(boldRed(scanner.Text()))

		}
		if err := scanner.Err(); err != nil {
			log.Println("reading standard err:", err)
		}
	}()

	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	if err := cmd.Wait(); err != nil {
		switch err.(type) {
		default:
			log.Fatal(err)
		case *exec.ExitError:
			log.Printf(boldRed("Error with command: %s"), err)
		}
	}
}

func main() {
	watcher, err := inotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}

	err = watcher.Watch(".")
	if err != nil {
		log.Fatal(err)
	}

	for {
		select {
		case ev := <-watcher.Event:
			processEvent(ev)
		case err := <-watcher.Error:
			log.Println("error:", err)
		}
	}
}
