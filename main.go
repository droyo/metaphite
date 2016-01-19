package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/droyo/metaphite/accesslog"
	"github.com/droyo/metaphite/config"
)

var (
	addr = flag.String("http", "", "address to listen on")
	file = flag.String("c", "", "configuration file")
)

func main() {
	log.SetFlags(0)
	flag.Parse()
	if *file == "" {
		log.Print("config file (-c) is required")
		flag.PrintDefaults()
		os.Exit(1)
	}
	if cfg, err := config.ParseFile(*file); err != nil {
		log.Fatalf("parse %s failed: %s", *file, err)
	} else {
		http.Handle("/render", accesslog.Handler(cfg, nil))
		if *addr == "" {
			*addr = cfg.Address
		}
	}
	status := make(chan error)
	go func() {
		status <- http.ListenAndServe(*addr, nil)
	}()
	log.Printf("listening on %s", *addr)
	if err := <-status; err != nil {
		log.Fatal(err)
	}
}
