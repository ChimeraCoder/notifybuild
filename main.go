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
	"sync"

	"github.com/fatih/color"
	"golang.org/x/exp/inotify"
)

var triggeredCommands = []*exec.Cmd{}

var keyExtensions = []string{".go", ".tmpl", ".html", ".js", ".jsx"}

// Signal to kill the currently running command (if any)
// Buffer the channel so sending a kill signal does not block
// even if there are no commands to read it
var killCmdSig chan struct{} = make(chan struct{}, 1)

// Signal to kill the entire program
var killSig chan struct{} = make(chan struct{})

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

func processEvent(event *inotify.Event, watcher *inotify.Watcher, killCmdSig <-chan struct{}) (killed bool) {
	// If the watcher is closed, e will be nil
	if event == nil {
		return
	}

	if !triggerRebuild(event.Name) {
		return
	}

	switch event.Mask {

	case inotify.IN_MODIFY, inotify.IN_CREATE, inotify.IN_CLOSE_WRITE:
		watcher.Close()
		cyan("Modified: %s ... rebuilding", event.Name)
		killed = rebuild(killCmdSig)
	}
	return
}

func rebuild(killSig <-chan struct{}) (killed bool) {
	// TODO check if all commands completed successfully
	wg := &sync.WaitGroup{}

	killSigChans := make([]chan<- struct{}, len(triggeredCommands))
	for i, cmd := range triggeredCommands {
		killCmdSig := make(chan struct{}, 1)
		killSigChans[i] = killCmdSig
		wg.Add(1)
		go backgroundTask(cmd, killCmdSig, wg)
	}

	allDone := make(chan struct{})
	go func() {
		wg.Wait()
		allDone <- struct{}{}
	}()

	select {
	case <-allDone:
		return
	case <-killSig:
		red("Received kill signal while processing events - sending to all others")
		wg.Add(1)
		killed = true
		for _, ch := range killSigChans {
			ch <- struct{}{}
		}
		wg.Done()
	}

	return
}

func backgroundTask(cmd *exec.Cmd, killed <-chan struct{}, wg *sync.WaitGroup) {
	var done chan struct{} = make(chan struct{})
	defer wg.Done()
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
				boldRed("Error with command: %s", err)
			}
		} else {
			cyan("Completed: %s", strings.Join(cmd.Args, " "))
		}
		done <- struct{}{}
	}()

	select {
	case <-killed:
		red("Received kill signal")
		cmd.Process.Kill()
		return
	case <-done:
		// Finished command
	}
}

func main() {

	const WatchDir = "."
	var watcher *inotify.Watcher
	var err error

	conf, err := parseConfig()
	if err != nil {
		log.Fatal(err)
	}

	color.Set(color.FgYellow)
	log.Printf("Running %d tasks", len(conf.Tasks))
	color.Unset()
	for name, task := range conf.Tasks {
		cmdArgs := strings.Fields(task.Cmd)
		cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
		color.Set(color.FgYellow)
		log.Printf("Registering %s (%s)", name, task.Cmd)
		color.Unset()
		triggeredCommands = append(triggeredCommands, cmd)
	}

	color.Set(color.FgCyan)
	dir, _ := filepath.Abs(filepath.Dir("."))
	log.Printf("Monitoring directory %s", dir)
	color.Unset()

	// Handle ^C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for _ = range c {
			// ^C has been sent
			red("Sending kill signal")
			killCmdSig <- struct{}{}
			red("Sent kill signal")
		}
	}()

	for {
		watcher, err = inotify.NewWatcher()
		if err != nil {
			log.Fatal(err)
		}
		err = watcher.Watch(WatchDir)
		if err != nil {
			log.Fatal(err)
		}
		select {
		case ev := <-watcher.Event:
			// Remove the watcher while the event is being handled
			// Otherwise "gofmt -w ." will trigger another event!
			killed := processEvent(ev, watcher, killCmdSig)
			if killed {
				return
			}
		case err := <-watcher.Error:
			log.Println("error:", err)
		case _ = <-killCmdSig:
			// This will only execute if a kill signal is sent
			// while no event is being processed
			red("Received kill signal - no event is currently being processed")
			return

		}
	}
}
