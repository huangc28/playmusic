package main

import (
	gorillaws "github.com/gorilla/websocket"
)

// Constructing chatrooms architecture
//
// 1. When message is emitting to a non-existing chatroom, we will register this chatroom to a key value hash.
//    with chatroom uuid as key, connection information as the value. ex:
//
// ChannelMap is a key/value hash we use to persist channel and all it's subscriptions.
// route: ws/chatroomA
//
//   If someone emit message to chatroomA and chatroomA does not exist, we then create a chatroom in ChannelMap:
//
//   {
//      ChatroomA: {
//        []Client:
//  	}
//   }
//
//  Client contains the following information:
//
//    - conn ---> conn instance of *websocket.Conn we retrieved from `upgrader.Upgrade(w, r, nil)`.
//    - send ---> chan of type []byte with length of 256. We use this method to send message.
//
// 2. All client subsciption to the chatroom will be persisted  in `[]Client`.

// Client indicates a connection user. The application receive and process
// message via Client instances. Each connection would spawn two goroutines
// to read / write messages in the background. Client sent message can be
// broadcasted to the chatroom vis `hub.broadcast` chan.
type Client struct {
	hub *Hub

	conn *gorillaws.Conn

	// sends grabs a message given and sends to the client.
	send chan []byte
}

// We will first implement single chatroom structure.
type Hub struct {
	// clients holds all registered connections of this chatroom. This map
	// indicates if or not a given connection(user) is still in the chatroom.
	clients map[*Client]bool

	// message received from one of the client needs to broadcast to other clients
	// in the chatroom. The implementation detail of broadcasting is to iterate through `clients`
	// to retrieve each client to push message to `send` channel.
	broadcast chan []byte

	// register a new connection in clients map. When client
	// dial the socket url, a new client will be sent to the
	// channel.
	register chan *Client

	// unregister a client from clients map. It means that the client has disconnected due to
	// unexpected error or intentional actions.
	unregister chan *Client
}

func newHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			{
				// Register a new client so that we can handle inbound / outbound
				// of that message later.
				if _, exists := h.clients[client]; !exists {
					h.clients[client] = true
				}
			}

		case client := <-h.unregister:
			{
				if _, exists := h.clients[client]; exists {
					// Close client connection.
					client.conn.Close()

					// Close underlying send channel since the client won't send out any message.
					close(client.send)

					// Remove client from map.
					delete(h.clients, client)

				}
			}
		}
	}
}
