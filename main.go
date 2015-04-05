package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fatih/color"
	"golang.org/x/exp/inotify"
	"gopkg.in/yaml.v2"
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
	var wg sync.WaitGroup

	killSigChans := make([]chan<- struct{}, len(triggeredCommands))
	for i, cmd := range triggeredCommands {
		killCmdSig := make(chan struct{})
		killSigChans[i] = killCmdSig
		wg.Add(1)
		go backgroundTask(cmd, killCmdSig, wg)
	}

	go func() {
		for {
			select {
			case <-killSig:
				wg.Add(1)
				killed = true
				for _, ch := range killSigChans {
					ch <- struct{}{}
				}
			}
		}
	}()

	wg.Wait()
	return
}

func backgroundTask(cmd *exec.Cmd, killed <-chan struct{}, wg sync.WaitGroup) {
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
