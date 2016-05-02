package storage

import (
    "os"
    "testing"
    "time"
)

func TestWithTTL(t *testing.T) {
    s, err := NewStorage(Settings{
        Url: os.Getenv("REDIS_URL"),
        KeyTTL: 3,
    })

    if err != nil {
        t.Fatal(err)
    }

    s.Set("key", "value")

    if !s.HasKey("key") {
        t.Fatal("Key not found.")
    }

    time.Sleep(5 * time.Second)

    if s.HasKey("key") {
        t.Fatal("Key found. It should expire.")
    }
}

func TestWithoutTTL(t *testing.T) {
    s, err := NewStorage(Settings{
        Url: os.Getenv("REDIS_URL"),
        KeyTTL: 0,
    })

    if err != nil {
        t.Fatal(err)
    }

    s.Set("key", "value")

    if !s.HasKey("key") {
        t.Fatal("Key not found.")
    }
}
