package api

import (
	"context"
	"os"

	"github.com/aisalamdag23/simple-websocket-app/pubsub"
	redis "github.com/go-redis/redis/v8"
	log "github.com/sirupsen/logrus"
)

type (
	Listener struct {
		Context context.Context
		PS      *pubsub.PubSub
		Hub     *Hub
	}
	Message struct {
		Value string `json:"message"`
	}
)

func (l *Listener) PubSubListener() {
	log.Info("PubSub Listening...")
	rpubsub := l.PS.Client.PSubscribe(l.Context, os.Getenv("REDIS_TOPIC"))
	_, err := rpubsub.Receive(l.Context)
	if err != nil {
		log.Error("pubsub receive error:", err)
	}
	go l.reader(rpubsub)
}

func (l *Listener) reader(rpubsub *redis.PubSub) {
	for msg := range rpubsub.Channel() {

		bcDetails := broadcastDetails{
			message: []byte(msg.Payload),
			topic:   "TEST_TOPIC",
		}

		l.Hub.broadcast <- bcDetails

	}
}
