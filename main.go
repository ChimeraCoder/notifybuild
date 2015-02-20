package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"golang.org/x/exp/inotify"
)

var triggeredCommands = []*exec.Cmd{}

var keyExtensions = []string{".go", ".tmpl", ".html", ".js"}

// Signal to kill the currently running command (if any)
var killCmdSig chan struct{} = make(chan struct{})

// Signal to kill the entire program
var killSig chan struct{} = make(chan struct{})

//var cyan = color.New(color.FgCyan).SprintFunc()
//var red = color.New(color.FgRed).Add(color.Bold).SprintFunc()
//var boldRed = color.New(color.FgRed).Add(color.Bold).SprintFunc()

func cyan(format string, args ...interface{}) {
	color.Set(color.FgCyan)
	log.Printf(format, args...)
	color.Unset()
}

func boldCyan(format string, args ...interface{}) {
	color.Set(color.FgCyan, color.Bold)
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

func green(format string, args ...interface{}) {
	color.Set(color.FgGreen)
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

func processEvent(event *inotify.Event, killCmdSig <-chan struct{}) (killed bool) {

	if !triggerRebuild(event.Name) {
		return
	}

	switch event.Mask {

	case inotify.IN_MODIFY:
		cyan("Modified: %s ... rebuilding", event.Name)

		killed = rebuild(killCmdSig)
	}
	return
}

func rebuild(killCmdSig <-chan struct{}) (killed bool) {
	// Did all commands complete successfully?

	var success = true
	var done chan struct{} = make(chan struct{})

	for _, cmd := range triggeredCommands {
		// Clone the command since it can only be used once
		cmd = exec.Command(cmd.Args[0], cmd.Args[1:]...)
		green("Running %+v", cmd.Args)
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

		go func() {
			if err := cmd.Wait(); err != nil {
				switch err.(type) {
				default:
					log.Fatal((color.New(color.FgRed).SprintFunc())(err))
				case *exec.ExitError:
					success = false
					boldRed("Error with command: %s", err)
				}
			} else {
				cyan("Completed: %s", strings.Join(cmd.Args, " "))
			}
			done <- struct{}{}
		}()
		select {
		case <-killCmdSig:
			boldRed("Received kill signal")
			killed = true
			cmd.Process.Kill()
			return
		case <-done:
			// Finished command
		}
	}

	if success {
		boldCyan("Successful build!")
	}
	return
}

func init() {
	args := os.Args

	// Default to a single "go build"
	if len(args) == 1 {
		triggeredCommands = []*exec.Cmd{exec.Command("go", "build")}
		return
	}

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

    const WatchDir = "."

	watcher, err := inotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}


	color.Set(color.FgCyan)
	dir, _ := filepath.Abs(filepath.Dir("."))
	log.Printf("Monitoring directory %s", dir)
	color.Unset()

    // Handle ^C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
    go func(){
        for _ = range c {
            // ^C has been sent
			boldRed("Sending kill signal")
			killCmdSig <- struct{}{}
			boldRed("Sent kill signal")
        }
    }()

    for {
        err = watcher.Watch(WatchDir)
        if err != nil {
            log.Fatal(err)
        }
        select {
        case ev := <-watcher.Event:
            // Remove the watcher while the event is being handled
            // Otherwise "gofmt -w ." will trigger another event!
            watcher.RemoveWatch(WatchDir)
            killed := processEvent(ev, killCmdSig)
            if killed {
                return
            }
        case err := <-watcher.Error:
            log.Println("error:", err)
        case _ = <-killCmdSig:
            // This will only execute if a kill signal is sent
            // while no event is being processed
            boldRed("Received kill signal - no event is currently being processed")
            return

        }
    }
}
