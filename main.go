package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/gorilla/mux"
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

func main() {
	var wait time.Duration
	flag.Parse()

	// We need to initialize hub in a gorouting to handle following
	// jobs in the background:
	//   - client (connection) register.
	//   - client (connection) unregister.
	//   - broadcast message to everyone(other clients) in the chatroom.
	hub := newHub()
	go hub.Run()

	r := mux.NewRouter()

	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	r.HandleFunc("/", serveHome).Methods("POST")
	r.HandleFunc("/audio-source", getAudioSourceFromYTUrl).Methods("GET")
	r.HandleFunc("/ws/chatroom", serveChatroom).Methods("GET")
	r.HandleFunc("/ws/music-stream", func(w http.ResponseWriter, r *http.Request) {
		serveMusicStream(hub, w, r)
	}).Methods("GET")

	srv := &http.Server{
		Handler:      r,
		Addr:         ":8080",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	// Serve http request handler
	// Serve websocket request handler
	//http.HandleFunc("/", serveHome)
	// http.HandleFunc("/", getAudioSourceFromYTUrl)
	// http.HandleFunc("/ws/chatroom", serveChatroom)
	// http.HandleFunc("/ws/music-stream", func(w http.ResponseWriter, r *http.Request) {
	// 	serveMusicStream(hub, w, r)
	// })

	// http.HandleFunc("/ws/music-stream-v1", func(w http.ResponseWriter, r *http.Request) {
	// 	websocket.Handler(HandleWebSockets).ServeHTTP(w, r)
	// })

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()

	c := make(chan os.Signal, 1)
	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(c, os.Interrupt)

	// Block until we receive our signal.
	<-c

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	srv.Shutdown(ctx)
	// Optionally, you could run srv.Shutdown in a goroutine and block on
	// <-ctx.Done() if your application should wait for other services
	// to finalize based on context cancellation.
	log.Println("shutting down")
	os.Exit(0)
}
