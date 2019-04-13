package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// url with status
type uws struct {
	url    string
	status int
}

var (
	maxConns        = 2
	maxCheckWorkers = 2
)

var (
	connworkers  = make(chan *connWorker, maxConns)
	checkworkers = make(chan *checkWorker, maxCheckWorkers)
	check        = make(chan string, 10000)
	checked      = make(chan uws, 10000)
)

func main() {
	ln, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("couldn't start tcp server: %s", err)
	}
	log.Printf("Listening for tcp connections on :8080")

	go writer()
	go dispatcher()

	for i := 0; i < maxConns; i++ {
		connworkers <- &connWorker{}
	}

	for {
		w := <-connworkers
		conn, _ := ln.Accept()
		w.handle(conn)
	}
}

type connWorker struct{}

func (w *connWorker) handle(c net.Conn) {
	defer c.Close()
	defer func() { connworkers <- w }()

	for {
		u, err := bufio.NewReader(c).ReadString('\n')
		if err == io.EOF {
			return
		}
		if err != nil {
			return
		}
		u = strings.TrimSuffix(u, "\n")
		if _, err := url.Parse(u); err != nil {
			return
		}

		check <- u
	}
}

func dispatcher() {
	// start up the checking workers
	for i := 0; i < maxCheckWorkers; i++ {
		w := &checkWorker{
			id: i,
			c:  make(chan string),
		}
		go w.start()
	}

	log.Printf("dispatcher starting")

	for {
		log.Printf("dispatcher waiting for a job")
		select {
		case u := <-check:
			log.Printf("waiting for check worker: %s", u)
			w := <-checkworkers // wait for a worker
			log.Printf("checkworker %d found", w.id)
			w.c <- u // send them a url to check
		}
	}

	log.Printf("dispatcher ending")
}

type checkWorker struct {
	id int
	c  chan string
}

func (c *checkWorker) start() {
	for {
		checkworkers <- c // signal that we're ready for work

		select {
		case u := <-c.c:
			status := c.check(u)      // check the url
			checked <- uws{u, status} // push to the results channel for writing
		}
	}
}

func (c *checkWorker) check(url string) int {
	fmt.Printf("checking url %s\n", url)
	resp, err := http.Head(url)
	if err != nil {
		return -1
	}

	return resp.StatusCode
}

func writer() {
	fo, err := os.Create("urls.txt")
	if err != nil {
		log.Fatalf("couldn't open output file for writing: %s", err)
	}
	defer fo.Close()

	w := bufio.NewWriter(fo)
	for {
		url := <-checked
		w.WriteString(fmt.Sprintf("%s,%d\n", url.url, url.status))

		// TODO: move flush to be periodic instead of after every string
		w.Flush()
	}
}
