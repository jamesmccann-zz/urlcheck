package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func main() {
	ln, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("couldn't start tcp server: %s", err)
	}
	log.Printf("Listening for tcp connections on :8080")

	for {
		conn, _ := ln.Accept()

		go func() {
			defer conn.Close()
			urlsFromReader(conn)
		}()
	}
}

func urlsFromReader(r io.Reader) {
	for {
		u, err := bufio.NewReader(r).ReadString('\n')
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

		status, err := check(u)
		if err != nil {
			return
		}

		fmt.Printf("url received: %s, status: %d\n", u, status)
	}
}

func check(url string) (int, error) {
	resp, err := http.Head(url)
	if err != nil {
		return -1, err
	}

	return resp.StatusCode, nil
}
