package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/golang/glog"
	"golang.org/x/net/websocket"
)

const ExampleOutputFile = "./example.m4a"

var useWebsockets = flag.Bool("websockets", false, "Whether to use websockets")

type Message struct {
	Id      int    `json:"id,omitempty"`
	Message string `json:"message,omitempty"`
}

// Client.
func main() {
	flag.Parse()

	out, err := os.Create(ExampleOutputFile)

	if err != nil {
		log.Fatalf("failed to create example file %v", err)
	}

	defer out.Close()

	if *useWebsockets {
		ws, err := websocket.Dial("ws://localhost:8080/", "", "http://localhost:8080")

		if err != nil {
			log.Fatalf("failed to dial socket server %v", err)
		}

		for {
			var chunkByte []byte
			if err := websocket.Message.Receive(ws, &chunkByte); err != nil {
				if errors.Is(err, websocket.ErrFrameTooLarge) {
					log.Fatalf("frame too large %v", err)
				}

				if err == io.EOF {
					glog.Infof("end of file %v", err.Error())

					break
				}

				glog.Fatal(err)
			}

			// Define write byte data to file.
			reader := bytes.NewReader(chunkByte)
			io.Copy(out, reader)
		}
	} else {
		glog.Info("Sending request...")
		req, err := http.NewRequest("GET", "http://localhost:8080", nil)
		if err != nil {
			glog.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			glog.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			glog.Fatalf("Status code is not OK: %v (%s)", resp.StatusCode, resp.Status)
		}

		dec := json.NewDecoder(resp.Body)
		for {
			var m Message
			err := dec.Decode(&m)
			if err != nil {
				if err == io.EOF {
					break
				}
				glog.Fatal(err)
			}
			glog.Infof("Got response: %+v", m)
		}
	}

	glog.Infof("Server finished request...")
}
