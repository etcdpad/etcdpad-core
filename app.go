package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/etcdpad/etcdpad-core/engine"
)

func main() {
	var (
		port        int
		logpath     string
		defaultpath string
		stdout      bool
	)

	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	if runtime.GOOS == "windows" {
		defaultpath = fmt.Sprintf("%s\\%s", dir, "oplog.log")
	} else {
		defaultpath = fmt.Sprintf("%s/%s", dir, "oplog.log")
	}

	flag.IntVar(&port, "port", 58518, "websocket listen port")
	flag.StringVar(&logpath, "logpath", defaultpath, "websocket client operate log file directory")
	flag.BoolVar(&stdout, "stdout", true, "epad log output to stdout")

	flag.Parse()

	log.Printf("epad log filepath: %s\n", logpath)
	log.Printf("epad websocket port: %d\n", port)
	log.Printf("epad log output to stdout: %+v\n", stdout)

	eg := engine.NewEngine(port, logpath, stdout)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
	<-signals

	eg.ShutDown()
}
