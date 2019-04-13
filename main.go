package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
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
		go w.handle(conn)
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

	for {
		select {
		case u := <-check:
			w := <-checkworkers // wait for a worker
			w.c <- u            // send them a url to check
		}
	}
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
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

			status := c.check(ctx, u) // check the url
			checked <- uws{u, status} // push to the results channel for writing

			cancel()
		}
	}
}

func (c *checkWorker) check(ctx context.Context, u string) int {
	fmt.Printf("[cw %d] checking url %s\n", c.id, u)
	if _, err := url.Parse(u); err != nil {
		return -1
	}

	req, err := http.NewRequest(http.MethodHead, u, nil)
	if err != nil {
		return -1
	}

	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
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
