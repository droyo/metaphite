package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"os"

	"github.com/droyo/meta-graphite/accesslog"
	"github.com/droyo/meta-graphite/config"
)

var (
	addr = flag.String("http", "", "address to listen on")
	file = flag.String("c", "", "configuration file")
)

func main() {
	var srv httputil.ReverseProxy

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
		srv.Director = cfg.MapRequest
		if *addr == "" {
			*addr = cfg.Address
		}
		if cfg.InsecureHTTPS {
			srv.Transport = &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			}
		}
	}
	http.Handle("/render", accesslog.Handler(&srv, nil))
	status := make(chan error)
	go func() {
		status <- http.ListenAndServe(*addr, nil)
	}()
	log.Printf("listening on %s", *addr)
	log.Fatal(<-status)
}
