package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "log"
    "net/http"
    "os"
    "strconv"

    "github.com/gorilla/mux"

    "github.com/eventials/frodo/broker"
    "github.com/eventials/frodo/storage"
    "github.com/eventials/frodo/sse"
)

type ChannelStats struct {
    ClientCount int `json:"client_count"`
}

type Stats struct {
    PoolCount int `json:"pool_count"`
    ChannelCount int `json:"channel_count"`
    ClientCount int `json:"client_count"`
    Channels map[string]ChannelStats `json:"channels"`
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
    fmt.Fprint(w, "Frodo")
}

func run(cacheTTL int, cacheUrl, brokerUrl, brokerQueue, bindAddress string) {
    cache, err := storage.NewStorage(storage.Settings{
        Url: cacheUrl,
        KeyTTL: cacheTTL,
    })

    if err != nil {
        log.Fatalf("Can't connect to storage: %s\n", err)
    }

    log.Println("Connected to storage.")
    defer cache.Close()

    es := sse.NewEventSource(sse.Settings{
        OnClientConnect: func (es *sse.EventSource, c *sse.Client) {
            // When a client connect for the first time, we send the last message.
            if msg, ok := es.GetLastMessage(c.Channel()); ok {
                log.Println("Sending last message to new client.")
                c.SendMessage(msg)
            }
        },
        OnChannelCreate: func (es *sse.EventSource, channelName string) {
            // Load the last message from cache to memory.
            if msg, err := cache.Get(channelName); err == nil {
                log.Println("Loading last message from cache.")
                es.SetLastMessage(channelName, msg)
            }
        },
    })

    log.Println("Event Source started.")
    defer es.Shutdown()

    b, err := broker.NewBroker(broker.Settings{
        Url: brokerUrl,
        ExchangeName: brokerQueue,
        OnMessage: func (eventMessage broker.BrokerMessage) {
            c := eventMessage.Channel
            msg := string(eventMessage.Data[:])
            cache.Set(c, msg)
            es.SendMessage(c, msg)
        },
    })

    if err != nil {
        log.Fatalf("Can't connect to broker: %s\n", err)
    }

    log.Println("Connected to broker.")
    defer b.Close()

    err = b.StartListen()

    if err != nil {
        log.Fatalf("Can't receive messages: %s\n", err)
    }

    router := mux.NewRouter()

    router.HandleFunc("/appstatus", func (w http.ResponseWriter, r *http.Request) {
        cacheOK := cache.Ping()
        brokerOK := b.Ping()
        statusOK := cacheOK && brokerOK

        if !statusOK {
            w.WriteHeader(http.StatusInternalServerError)
        }

        w.Write([]byte(fmt.Sprintf("status:%t,cache:%t,broker:%t", statusOK, cacheOK, brokerOK)))
    })

    router.HandleFunc("/api/stats", func (w http.ResponseWriter, r *http.Request) {
        channels := es.Channels()
        stats := Stats{
            cache.ConnectionCount(),
            len(channels),
            es.ConnectionCount(),
            make(map[string]ChannelStats),
        }

        for _, name := range channels {
            stats.Channels[name] = ChannelStats{
                es.ConnectionCountPerChannel(name),
            }
        }

        dump, err := json.Marshal(&stats)

        if err == nil {
            w.Header().Set("Content-Type", "application/json")
            w.Write(dump)
        } else {
            w.WriteHeader(http.StatusInternalServerError)
        }
    })

    router.Handle("/{channel:[a-z0-9-_/]+}", es)
    router.HandleFunc("/", indexHandler)

    log.Printf("Server started at %s.\n", bindAddress)
    http.ListenAndServe(bindAddress, router)
}

func defaultValue(a, b string) string {
    if len(a) == 0 {
        return b
    }

    return a
}

func main() {
    var ttl int64
    var err error

    if ttl, err = strconv.ParseInt(defaultValue(os.Getenv("FRODO_TTL"), "60"), 10, 64); err != nil {
        ttl = 60
    }

    cacheUrl := flag.String("cache", defaultValue(os.Getenv("FRODO_CACHE"), "redis://127.0.0.1:6379/0"), "Cache URL.")
    cacheTTL := flag.Int("ttl", int(ttl), "Cache TTL in seconds.")
    brokerUrl := flag.String("broker", defaultValue(os.Getenv("FRODO_BROKER"), "amqp://"), "Broker URL.")
    brokerQueue := flag.String("queue", defaultValue(os.Getenv("FRODO_QUEUE"), "frodo"), "Broker Queue.")
    bindAddress := flag.String("bind", defaultValue(os.Getenv("FRODO_BIND"), ":3000"), "Bind Address.")

    flag.Parse()

    run(*cacheTTL, *cacheUrl, *brokerUrl, *brokerQueue, *bindAddress)
}
