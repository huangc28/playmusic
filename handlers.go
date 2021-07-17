package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/golang/glog"
	"github.com/kkdai/youtube/v2"
	"golang.org/x/net/http/httpproxy"
	"golang.org/x/net/websocket"
)

const ExampleYTUrl = "https://www.youtube.com/watch?v=07d2dXHYb94&ab_channel=SoutheasternGuideDogs"

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
		glog.Info("failed to get stream context %s", err.Error())

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
