package api

import (
	log "github.com/sirupsen/logrus"
)

type broadcastDetails struct {
	topic          string
	message        []byte
	specificClient *Client
}

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// Subscribed clients.
	clients map[string]map[string]*Client

	// Outbound messages to the clients.
	broadcast chan broadcastDetails

	// Subscribe requests from the clients.
	subscribe chan *Client

	// Unsubscribe requests from clients.
	unsubscribe chan *Client
}

func NewHub() *Hub {
	return &Hub{
		broadcast:   make(chan broadcastDetails),
		subscribe:   make(chan *Client),
		unsubscribe: make(chan *Client),
		clients:     make(map[string]map[string]*Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.subscribe:
			log.WithFields(log.Fields{
				"client_conn": client.conn.RemoteAddr().String(),
				"id":          client.id,
			}).Info("subscribe to " + client.topic + " client id " + client.id)

			c := h.clients[client.topic]
			if c == nil {
				c = make(map[string]*Client)
				h.clients[client.topic] = c
			}
			c[client.id] = client

		case client := <-h.unsubscribe:
			log.WithFields(log.Fields{
				"client_conn": client.conn.RemoteAddr().String(),
				"id":          client.id,
			}).Info("unsubscribe from " + client.topic + " client id " + client.id)

			if h.clients[client.topic] != nil {
				if _, ok := h.clients[client.topic][client.id]; ok {
					delete(h.clients[client.topic], client.id)
					// close(client.send)
					if len(h.clients[client.topic]) == 0 {
						// This was last client in the topic, delete the topic
						delete(h.clients, client.topic)
					}
				}
			}

		case bdMsg := <-h.broadcast:
			// handle broadcasting to specific client - mostly used for special cases like unexpected close errors or invalid data request
			if bdMsg.specificClient != nil {
				client := bdMsg.specificClient
				log.WithFields(log.Fields{
					"client_conn": client.conn.RemoteAddr().String(),
					"id":          client.id,
				}).Info("broadcasting to specific client connection")

				select {
				case client.send <- bdMsg.message:
				default:
					close(client.send)
				}
			}
			if bdMsg.topic != "" && h.clients[bdMsg.topic] != nil {
				for id, client := range h.clients[bdMsg.topic] {
					log.WithFields(log.Fields{
						"id":          id,
						"client_conn": client.conn.RemoteAddr().String(),
						"topic":       bdMsg.topic,
					}).Info("broadcasting to client connection")

					select {
					case client.send <- bdMsg.message:
					default:
						// close(client.send)
						delete(h.clients[bdMsg.topic], id)
					}
				}
				if len(h.clients[bdMsg.topic]) == 0 {
					// The topic was emptied while broadcasting.  Delete the topic.
					delete(h.clients, bdMsg.topic)
				}
			}
		}
	}
}
