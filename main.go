package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"golang.org/x/exp/inotify"
)

var triggeredCommands = []*exec.Cmd{}

var keyExtensions = []string{".go", ".tmpl", ".html", ".js"}

//var cyan = color.New(color.FgCyan).SprintFunc()
//var red = color.New(color.FgRed).Add(color.Bold).SprintFunc()
//var boldRed = color.New(color.FgRed).Add(color.Bold).SprintFunc()

func cyan(format string, args ...interface{}) {
	color.Set(color.FgCyan)
	log.Printf(format, args...)
	color.Unset()
}

func boldRed(format string, args ...interface{}) {
	color.Set(color.FgRed, color.Bold)
	log.Printf(format, args...)
	color.Unset()
}

func red(format string, args ...interface{}) {
	color.Set(color.FgRed)
	log.Printf(format, args...)
	color.Unset()
}

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
		cyan("Modified: %s ... rebuilding", event.Name)

		rebuild()
	}
}

func rebuild() {
	for _, cmd := range triggeredCommands {
		cmd = exec.Command("go", "build")
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
				red("reading standard input:", err)
			}
		}()

		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				red(scanner.Text())

			}
			if err := scanner.Err(); err != nil {
				red("reading standard err:", err)
			}
		}()

		if err := cmd.Start(); err != nil {
			log.Fatal(err)
		}

		if err := cmd.Wait(); err != nil {
			switch err.(type) {
			default:

				log.Fatal((color.New(color.FgRed).SprintFunc())(err))
			case *exec.ExitError:
				boldRed("Error with command: %s", err)
			}
		} else {
			cyan("Successful build!")
		}
	}
}

func init() {
	args := os.Args
	commandStrings := strings.Split(args[1], "&&")

	color.Set(color.FgYellow)
	log.Printf("Running %d commands: %+v", len(commandStrings), commandStrings)
	color.Unset()
	for _, command := range commandStrings {
		commandArgs := strings.Fields(command)
		cmd := exec.Command(commandArgs[0], commandArgs[1:]...)
		triggeredCommands = append(triggeredCommands, cmd)
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

	color.Set(color.FgCyan)
	dir, _ := filepath.Abs(filepath.Dir("."))
	log.Printf("Monitoring directory %s", dir)
	color.Unset()
	for {
		select {
		case ev := <-watcher.Event:
			processEvent(ev)
		case err := <-watcher.Error:
			log.Println("error:", err)
		}
	}
}
