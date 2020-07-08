package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/peterbourgon/ff"
)

func main() {

	fs := flag.NewFlagSet("wind-server", flag.ExitOnError)
	var (
		refreshWebhook = fs.String("webhook", "", "Refresh Webhook")
	)
	ff.Parse(fs, os.Args[1:], ff.WithEnvVarNoPrefix())

	n, err := InitNoaa(*refreshWebhook)
	if err != nil {
		log.Fatal("Enable to load existing grib files", err)
	}

	router := InitServer(n)
	log.Println("Start server on port 8090")
	log.Fatal(http.ListenAndServe(":8090", router))
}
