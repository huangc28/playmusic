package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/golang/glog"
	gorillaws "github.com/gorilla/websocket"
	"github.com/kkdai/youtube/v2"
	"golang.org/x/net/http/httpproxy"
	"golang.org/x/net/websocket"
)

// const ExampleYTUrl = "https://www.youtube.com/watch?v=07d2dXHYb94&ab_channel=SoutheasternGuideDogs"
// const ExampleYTUrl = "https://www.youtube.com/watch?v=_Yhyp-_hX2s&ab_channel=msvogue23"
const ExampleYTUrl = "https://www.youtube.com/watch?v=DEcxTQHH3Rc&ab_channel=RapMusicHD"

const ExampleStreamUrl = "https://r3---sn-ipoxu-un5e7.googlevideo.com/videoplayback?expire=1626183763&ei=80PtYLacId2D1d8PtLWL4A8&ip=220.136.67.137&id=o-AGChpWHBSX1Qf5oO0dTKLsuXFiRZ-AllB0KXhP77TqEp&itag=140&source=youtube&requiressl=yes&mh=o7&mm=31%2C26&mn=sn-ipoxu-un5e7%2Csn-oguelne7&ms=au%2Conr&mv=m&mvi=3&pl=21&initcwndbps=675000&vprv=1&mime=audio%2Fmp4&ns=KFa9Z5su7_cvDgzRDA2Sh2gG&gir=yes&clen=69399190&dur=4288.121&lmt=1611232081205090&mt=1626161810&fvip=3&keepalive=yes&fexp=24001373%2C24007246&c=WEB&txp=5431432&n=J6XpBfD44-ppCimPKv&sparams=expire%2Cei%2Cip%2Cid%2Citag%2Csource%2Crequiressl%2Cvprv%2Cmime%2Cns%2Cgir%2Cclen%2Cdur%2Clmt&lsparams=mh%2Cmm%2Cmn%2Cms%2Cmv%2Cmvi%2Cpl%2Cinitcwndbps&lsig=AG3C_xAwRgIhAJqRFQKTqHzHXZ4kZlUYo5yO70TdF2OfbleHGdtjUGjFAiEAtKUbJA-XUtJOaNXPoWQbOSolCTJPbJZKCCkYWuYOBcM%3D&sig=AOq0QJ8wRgIhAOCjZQFtiIiCXLxVZt3Jyy50lAa_v8f6IB-b5we42XuyAiEAmOC2K0ONQY72QChfo9pgAcmHy5AQhFOQTd3lUvwvVDI="

// ErrUnexpectedStatusCode is returned on unexpected HTTP status codes
type ErrUnexpectedStatusCode int

func (err ErrUnexpectedStatusCode) Error() string {
	return fmt.Sprintf("unexpected status code: %d", err)
}

func getHttpClient() *http.Client {
	proxyFunc := httpproxy.FromEnvironment().ProxyFunc()
	httpTransport := &http.Transport{
		Proxy: func(r *http.Request) (uri *url.URL, err error) {
			return proxyFunc(r.URL)
		},
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	return &http.Client{
		Transport: httpTransport,
	}
}

func getDlClient() *youtube.Client {
	return &youtube.Client{
		HTTPClient: getHttpClient(),
	}
}

type ErrMessage struct {
	Err string `json:"err_message"`
}

func HandleWebSockets(ws *websocket.Conn) {
	// Initialize requests with decoded url.
	httpClient := getDlClient()

	video, err := httpClient.GetVideo(ExampleYTUrl)

	if err != nil {
		websocket.JSON.Send(
			ws,
			ErrMessage{
				Err: fmt.Sprintf("failed to get video data: %s", err.Error()),
			},
		)
	}

	// The video contains multiple formats for video. We need to find the audio
	format := video.Formats.FindByItag(140)

	stream, size, err := httpClient.GetStream(
		video,
		format,
	)

	glog.Infof("content size %d", size)

	if err != nil {
		glog.Infof("failed to get stream context %s", err.Error())

		websocket.JSON.Send(ws, ErrMessage{
			Err: fmt.Sprintf("failed to get video stream data %s:", err.Error()),
		})

		return
	}

	defer stream.Close()

	written, err := io.Copy(ws, stream)

	glog.Infof("written byte %v", written)

	if err != nil {
		glog.Infof("Client stopped listening... %v", err.Error())

		return
	}
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	type Msg struct {
		OK bool
	}

	data := Msg{true}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		w.Write([]byte("request failed"))
	}
}

var upgrader = gorillaws.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func serveChatroom(w http.ResponseWriter, r *http.Request) {}

// We need to find a way to handle err response
func serveMusicStream(h *Hub, w http.ResponseWriter, r *http.Request) {
	// We need to upgrade incoming request to websocket.
	conn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		log.Printf("failed to upgrade connection %v", conn)

		return
	}

	// Initialize a client that consist of connection info
	client := &Client{
		hub:  h,
		conn: conn,
		send: make(chan []byte, 256),
	}

	client.hub.register <- client

	go streamMusic(client)
}

func streamMusic(c *Client) {
	defer func() {
		c.hub.unregister <- c
	}()

	// Retrieve music stream
	httpClient := getDlClient()

	video, err := httpClient.GetVideo(ExampleYTUrl)

	if err != nil {
		c.conn.WriteJSON(
			ErrMessage{
				Err: fmt.Sprintf("failed to get video data: %s", err.Error()),
			},
		)

		// void return the go-routine.
		return
	}

	// send chunks of byte data client
	format := video.Formats.FindByItag(140)
	stream, size, err := httpClient.GetStream(
		video,
		format,
	)

	if err != nil {
		glog.Infof("failed to get video stream %s", err.Error())

		c.conn.WriteJSON(
			ErrMessage{Err: fmt.Sprintf("failed to get video stream data %s:", err.Error())},
		)

		return
	}

	glog.Infof("content size %d", size)

	defer stream.Close()

	buf := make([]byte, 32*1024)
	var written int64 = 0

	for {
		rn, re := stream.Read(buf)

		// if no byte rn is read, nothings needs to be send to socket.
		if rn > 0 {
			if err := c.conn.WriteMessage(
				gorillaws.BinaryMessage,
				buf[0:rn],
			); err != nil {
				c.conn.WriteJSON(
					ErrMessage{
						Err: fmt.Sprintf("failed to send video byte chunk %s", err.Error()),
					},
				)

				break
			}

			written += int64(rn)
			glog.Infof("total read length %d", written)
		}

		if re != nil {
			if re == io.EOF {
				// We've reached end of file. shutdown the connection gracefully.
				glog.Info("end of file")

				break
			}

			c.conn.WriteJSON(
				ErrMessage{
					Err: fmt.Sprintf("failed to read video byte chunk %s", err.Error()),
				},
			)
		}
	}
}

func writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	w.WriteHeader(http.StatusBadRequest)
	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(
		ErrorResponse{
			Message: message,
		},
	)
}

type ErrorResponse struct {
	Message string `json:"message"`
}

func getAudioSourceFromYTUrl(w http.ResponseWriter, r *http.Request) {
	// Retrieve youtube url from request object
	qv := r.URL.Query()
	vurl, exists := qv["vurl"]

	if !exists {
		writeErrorResponse(
			w,
			http.StatusBadRequest,
			"youtube url not provided.",
		)

		return
	}

	// Check if vurl exists.
	httpClient := getDlClient()
	video, err := httpClient.GetVideo(vurl[0])

	if err != nil {
		writeErrorResponse(
			w,
			http.StatusBadRequest,
			err.Error(),
		)

		return
	}

	vf := video.Formats.FindByItag(140)

	surl, err := httpClient.GetStreamURL(video, vf)

	if err != nil {
		writeErrorResponse(
			w,
			http.StatusBadRequest,
			err.Error(),
		)

		return
	}

	// vf := video.Formats.FindByItag(140)
	// log.Printf("v url %v", video.Formats)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		StreamUrl string `json:"stream_url"`
	}{
		surl,
	})
}
