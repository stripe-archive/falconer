package main

import (
	"fmt"
	"log"
	"time"

	"github.com/boltdb/bolt"
	"github.com/stripe/veneur/ssf"
)

func main() {
	_, err := bolt.Open("mydb.db", 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		log.Fatal(err)
	}

	foo := ssf.SSFSpan{}

	fmt.Println("Hello Jacquard")
}
