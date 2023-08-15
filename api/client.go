package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/gorilla/websocket"
)

type (
	visibilityType string
	eventType      string
)

type closeMsg struct {
	Type    string `json:"type,omitempty"`
	Code    int64  `json:"code,omitempty"`
	Message string `json:"message"`
}

const (
	// Send pings to peer with this period.
	pingPeriod = 20

	VISIBILITY_PRIVATE  visibilityType = "private"
	VISIBILITY_PUBLIC   visibilityType = "public"
	EVENT_TRADES        eventType      = "trades"
	EVENT_ORDERBOOK     eventType      = "orderBook"
	EVENT_ORDER         eventType      = "order"
	EVENT_EXCHANGETRADE eventType      = "exchangeTrade"

	// transactions
	EVENT_WITHDRAWAL = "withdrawal"
	EVENT_DEPOSIT    = "deposit"

	CURRENCY_PAIR = "XRPPHP"
	SUBSCRIBE     = "subscribe"
	UNSUBSCRIBE   = "unsubscribe"

	PUBLIC_TOPIC_ORDERBOOK = "orderBook"
	PUBLIC_TOPIC_TRADES    = "trades"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	// unique ws generated client id identifier
	id string

	hub *Hub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	topic string
}

type Request struct {
	Event string `json:"event"`
	Topic string `json:"topic"`
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) readPump() {
	defer func() {
		c.hub.unsubscribe <- c
		c.conn.Close()
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Error("conn. "+c.conn.RemoteAddr().String()+" unexpected close error: ", err)
				closeMsg, _ := json.Marshal(closeMsg{Type: "unexpected close", Code: websocket.CloseMessage, Message: err.Error()})

				bcDetails := broadcastDetails{
					message:        closeMsg,
					topic:          "",
					specificClient: c,
				}

				c.hub.broadcast <- bcDetails
			}
			break
		}
		log.Info("conn. "+c.conn.RemoteAddr().String()+" request message: ", string(message))

		data := Request{}
		_ = json.Unmarshal([]byte(message), &data)

		subOrUnsub := data.Event
		reqTopic := strings.Split(data.Topic, "/")
		visibility := visibilityType(reqTopic[0])

		if visibility == VISIBILITY_PRIVATE {
			c.topic = reqTopic[1]
		} else if visibility == VISIBILITY_PUBLIC {
			c.topic = reqTopic[1]
			pair := reqTopic[2]

			if strings.ToUpper(pair) != CURRENCY_PAIR {
				msg := `{"event": "` + c.topic + `", "success": false, "reason": "Currency pair is not supported"}`
				log.Info("conn. "+c.conn.RemoteAddr().String()+" public response: ", msg)

				bcDetails := broadcastDetails{
					message:        []byte(msg),
					topic:          "",
					specificClient: c,
				}

				c.hub.broadcast <- bcDetails
				c.hub.unsubscribe <- c
				continue
			}
		}

		if subOrUnsub == SUBSCRIBE {
			c.hub.subscribe <- c
		} else if subOrUnsub == UNSUBSCRIBE {
			c.hub.unsubscribe <- c
		}
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				// The hub closed the channel.
				log.Info("hub closed the channel")
				returnMsg, err := json.Marshal(closeMsg{Type: "close message", Code: websocket.CloseMessage, Message: "Websocket connection closed"})
				if err != nil {
					log.Error("conn. "+c.conn.RemoteAddr().String()+" writePump return close message error: ", err)
				}
				err = c.conn.WriteMessage(websocket.CloseMessage, returnMsg)
				if err != nil {
					log.Error("conn. "+c.conn.RemoteAddr().String()+" write close message error: ", err)
				}
				return
			}

			err := c.conn.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				log.Error("conn. "+c.conn.RemoteAddr().String()+" write message error: ", err)
			}

		case <-ticker.C:
			pingMsg, _ := json.Marshal(closeMsg{Type: "ping"})
			if err := c.conn.WriteMessage(websocket.PingMessage, pingMsg); err != nil {
				log.Error("conn. "+c.conn.RemoteAddr().String()+" ticker write message error: ", err)
				return
			}
		}
	}
}

// ServeWs handles websocket requests from the peer.
func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error("upgrader error:", err)
		return
	}

	client := &Client{id: uuid.New().String(), hub: hub, conn: conn, send: make(chan []byte, 256)}

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.writePump()
	go client.readPump()
}
