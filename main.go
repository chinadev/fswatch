package main

import (
	"fmt"
	"github.com/howeyc/fsnotify"
	"github.com/jessevdk/go-flags"
	"github.com/shxsun/klog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	K = klog.NewLogger(nil, "")
)

func filter(watch chan *fsnotify.FileEvent) chan *fsnotify.FileEvent {
	modifyTime := make(map[string]int64)
	filterd := make(chan *fsnotify.FileEvent)
	go func() {
		for {
			ev := <-watch
			if isIgnore(filepath.Base(ev.Name)) {
				K.Debugf("IGNORE: %s", ev.Name)
				continue
			}
			// delete or rename has no modify time
			if ev.IsDelete() || ev.IsRename() {
				filterd <- ev
				continue
			}
			mt, err := getFileModTime(ev.Name)
			if err != nil {
				//K.Warnf("get file mod time failed: %s", err)
				continue
			}
			t := modifyTime[ev.Name]
			if mt == t {
				K.Debugf("SKIP: %s", ev.Name)
				continue
			}

			filterd <- ev
			modifyTime[ev.Name] = mt
		}
	}()
	return filterd
}

// the main goroutine
func watchEvent(watcher *fsnotify.Watcher, name string, args ...string) {
	go func() {
		for {
			err := <-watcher.Error          // ignore watcher error
			K.Warnf("watch error: %s", err) // No need to exit here
		}
	}()

	var cmd *exec.Cmd
	filterEvent := filter(watcher.Event)
	for {
		ev := <-filterEvent
		K.Info("Sense first:", ev)
	CHECK:
		select {
		case ev = <-filterEvent:
			K.Info("Sense again: ", ev)
			goto CHECK
		case <-time.After(notifyDelay):
		}
		// stop cmd
		if cmd != nil && cmd.Process != nil {
			K.Info("stop process")
			cmd.Process.Kill()
		}
		// start cmd
		cmd = exec.Command(name, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		K.Info(fmt.Sprintf("%s %5s %s", LeftRight, "START", LeftRight))
		err := cmd.Start()
		if err != nil {
			K.Error(err)
			continue
		} else {
			go func(cmd *exec.Cmd) {
				err := cmd.Wait()
				if err != nil {
					K.Error(fmt.Sprintf("%s %5s %s", LeftRight, "ERROR", LeftRight))
				} else {
					K.Info(fmt.Sprintf("%s %5s %s", LeftRight, "E N D", LeftRight))
				}
			}(cmd)
		}
	}
}

func NewWatcher(paths []string, name string, args ...string) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		K.Fatalf("fail to create new Watcher: %s", err)
	}

	K.Info("initial watcher")
	for _, path := range paths {
		K.Debugf("watch directory: %s", path)
		err = w.Watch(path)
		if err != nil {
			K.Fatal("fail to watch directory: %s", err)
		}
	}
	watchEvent(w, name, args...)
}

var (
	notifyDelay time.Duration = time.Second * 1
	LeftRight                 = strings.Repeat("-", 10)
)

var opts struct {
	Verbose bool `short:"v" long:"verbose" description:"Show verbose debug infomation"`
	Delay string `long:"delay" description:"Trigger event buffer time" default:"0.5s"`
}

func main() {
	parser := flags.NewParser(&opts, flags.Default | flags.PassAfterNonOption)
	args, err := parser.Parse()

	if err != nil {
		os.Exit(1)
	}
	if opts.Verbose {
		K.SetLevel(klog.LDebug)
	}
	notifyDelay, err = time.ParseDuration(opts.Delay)
	if err != nil {
		K.Warn(err)
		notifyDelay = time.Millisecond * 500
	}
	K.Debugf("delay time: %s", notifyDelay)

	if len(args) == 0 {
		fmt.Printf("Use %s --help for more details\n", os.Args[0])
		return
	}

	NewWatcher([]string{"."}, args[0], args[1:]...)
}
