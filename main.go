package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/golang/glog"
	"golang.org/x/net/websocket"
)

type Message struct {
	Id      int    `json:"id,omitempty"`
	Message string `json:"message,omitempty"`
}

// Heavily based on Kubernetes' (https://github.com/GoogleCloudPlatform/kubernetes) detection code.
var connectionUpgradeRegex = regexp.MustCompile(`(^|.*,\s*)upgrade($|\s*,)`)

func isWebsocketRequest(req *http.Request) bool {
	glog.Infof("connection header %v", req.Header.Get("Connection"))
	glog.Infof("upgrade header %v", req.Header.Get("Upgrade"))

	return connectionUpgradeRegex.MatchString(strings.ToLower(req.Header.Get("Connection"))) && strings.ToLower(req.Header.Get("Upgrade")) == "websocket"
}

func Handle(w http.ResponseWriter, r *http.Request) {
	// Handle websockets if specified.
	if isWebsocketRequest(r) {
		websocket.Handler(HandleWebSockets).ServeHTTP(w, r)
	} else {
		HandleHttp(w, r)
	}
	glog.Info("Finished sending response...")
}

func HandleHttp(w http.ResponseWriter, r *http.Request) {
	cn, ok := w.(http.CloseNotifier)
	if !ok {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.NotFound(w, r)
		return
	}

	// Send the initial headers saying we're gonna stream the response.
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	enc := json.NewEncoder(w)

	for i := 0; i < 5; i++ {
		select {
		case <-cn.CloseNotify():
			glog.Infof("Client stopped listening")
			return
		default:
			// Artificially wait a second between reponses.
			time.Sleep(time.Second)

			glog.Infof("Sending some data: %d", i)
			m := Message{
				Id:      i,
				Message: fmt.Sprintf("Sending you \"%d\"", i),
			}

			// Send some data.
			err := enc.Encode(m)
			if err != nil {
				glog.Fatal(err)
			}
			flusher.Flush()
		}
	}
}

// Server.
func main() {
	log.Println("DEBUG 1")

	flag.Parse()

	http.HandleFunc("/", Handle)

	log.Println("DEBUG 2")
	glog.Infof("Serving...")
	glog.Fatal(http.ListenAndServe(":8080", nil))
}
