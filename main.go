package main

import (
	"os"
	"time"
	"os/signal"
	"sync"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
)

const (
	version = "0.2"
	checkInterval = 5
)

var iptablesMutex = new(sync.Mutex)

func main() {

	var flagDebug = cli.BoolFlag{
		Name:  "debug, d",
		Usage: "enable debugging",
	}

	var flagIface = cli.StringFlag{
		Name:  "iface, i",
		Usage: "iface name",
	}

	app := cli.NewApp()
	app.Name = "float-ip"
	app.Usage = "Docker Linux Bridge Networking"
	app.Version = version
	app.Flags = []cli.Flag{
		flagDebug,
		flagIface,
	}
	app.Action = Run
	app.Run(os.Args)
}

func Run(ctx *cli.Context) error {
	if ctx.Bool("debug") {
		log.SetLevel(log.DebugLevel)
	}
	
	linkName := ctx.String("iface")
	if linkName == "" {
		linkName = mainLink()
	}
	
	initIptables()
	mainAddr, _ := getIfaceAddr(linkName)
	
	//cleaner
	c := make(chan os.Signal, 1)
	//The main process inside the container will receive SIGTERM, and after a grace period, SIGKILL.
	signal.Notify(c, os.Interrupt, os.Kill, syscall.SIGTERM) 
	cleanUp := func(){
		<- c
		iptablesMutex.Lock()
		clearIPtables(linkName, *mainAddr)
		iptablesMutex.Unlock()
		os.Exit(0)
	}
	//cleanup for pinic 
	defer cleanUp()
	go cleanUp()
	
	for {
		iptablesMutex.Lock()
		iptablesMaintainer(linkName, *mainAddr)
		iptablesMutex.Unlock()
		time.Sleep(checkInterval * time.Second)
	}
	return nil
}