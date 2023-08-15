package pubsub

import (
	"crypto/tls"
	"flag"
	"fmt"
	"os"

	"github.com/go-redis/cache/v8"
	"github.com/go-redis/redis/v8"
	log "github.com/sirupsen/logrus"
)

type PubSub struct {
	Client    *redis.Client
	ReadCache *cache.Cache
}

var PS PubSub

func NewPubSub() {
	uri := fmt.Sprintf("%s:%s", os.Getenv("REDIS_URI"), os.Getenv("REDIS_PORT"))
	pw := os.Getenv("REDIS_PW")

	var addr = flag.String("PubSub Server", uri, "Redis server address")
	cli := redis.NewClient(&redis.Options{Addr: *addr, Password: pw, DB: 0, TLSConfig: &tls.Config{
		MinVersion: tls.VersionTLS12,
	}})

	log.Info("Redis Server URI: ", uri)
	if cli == nil {
		log.Error("error: Redis Server client creation failed")
	}

	PS.Client = cli
	PS.ReadCache = NewRedisReadReplica()
}

func GetPubSub() *PubSub {
	return &PS
}

func NewRedisReadReplica() *cache.Cache {

	pw := os.Getenv("REDIS_PW")

	ring := redis.NewRing(&redis.RingOptions{
		Addrs: map[string]string{
			"main": fmt.Sprintf("%v:%v", os.Getenv("REDIS_READ_URI"), os.Getenv("REDIS_READ_PORT")),
		},
		DB:       0,
		Password: pw,
		PoolSize: 200,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	})

	return cache.New(&cache.Options{
		Redis: ring,
	})
}
