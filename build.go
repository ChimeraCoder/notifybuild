package main

import (
	"bufio"
	"log"
	"os/exec"
	"path/filepath"

	"github.com/fatih/color"
	"golang.org/x/exp/inotify"
)

var keyExtensions = []string{".go", ".tmpl", ".html", ".js"}

var boldRed = color.New(color.FgRed).Add(color.Bold).SprintFunc()

var ignoredFiles = map[string]struct{}{}

func triggerRebuild(filename string) bool {
	// Ignore hidden files
	if filepath.Base(filename)[:1] == "." {
		log.Printf("Ignoring hidden file %s", filename)
		return false
	}
	if _, ok := ignoredFiles[filepath.Clean(filename)]; ok {
		log.Printf("Ignoring event for %s", filename)
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
