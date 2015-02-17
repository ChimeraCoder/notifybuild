package main

import (
	"bufio"
	"log"
	"os/exec"
	"path/filepath"

	"github.com/fatih/color"
	"golang.org/x/exp/inotify"
)

var boldRed = color.New(color.FgRed).Add(color.Bold).SprintFunc()

var ignoredFiles = map[string]struct{}{}

func processEvent(event *inotify.Event) {
	// Ignore hidden files
	if filepath.Base(event.Name)[:1] == "." {
		return
	}
	if _, ok := ignoredFiles[filepath.Clean(event.Name)]; ok {
		log.Printf("Ignoring event for %s", event.Name)
		return
	}

	switch event.Mask {

	case inotify.IN_MODIFY:
		log.Printf(boldRed("Modified: %s"), event.Name)

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
			log.Println(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			log.Println("reading standard input:", err)
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Println(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			log.Println("reading standard err:", err)
		}
	}()

	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	if err := cmd.Wait(); err != nil {
		log.Fatal(err)
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

	f, err := filepath.Abs(filepath.Dir("."))
	if err != nil {
		log.Fatal(err)
	}
	binaryName := filepath.Base(f)
	ignoredFiles[filepath.Clean(binaryName)] = struct{}{}
	log.Printf("Ignoring binary file %s", filepath.Clean(binaryName))

	for {
		select {
		case ev := <-watcher.Event:
			log.Println("event:", ev)
			processEvent(ev)
		case err := <-watcher.Error:
			log.Println("error:", err)
		}
	}
}
