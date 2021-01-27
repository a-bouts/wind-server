package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/a-bouts/wind-server/api"
	"github.com/a-bouts/wind-server/providers"
	"github.com/peterbourgon/ff"
)

func main() {

	fs := flag.NewFlagSet("wind-server", flag.ExitOnError)
	var (
		noaa = fs.Bool("noaa", false, "Enable NOAA Provider")

		meteofrance      = fs.Bool("meteofrance", false, "Enable Meteo-France Provider")
		meteofranceToken = fs.String("meteofrance-token", "", "Meteo-France API Token")
	)
	ff.Parse(fs, os.Args[1:], ff.WithEnvVarNoPrefix())

	ps := make(map[string]providers.Provider)

	if *meteofrance {
		fmt.Println("Add Meteo-France Provider")
		m, err := providers.InitMeteoFranceProvider(*meteofranceToken)
		if err != nil {
			fmt.Println("Error Initializing Meteo France provider", err)
		} else {
			ps[m.GetId()] = m
		}
	}

	if *noaa {
		fmt.Println("Add Noaa Provider")
		n, err := providers.InitNoaaProvider()
		if err != nil {
			log.Fatal("Error Initializing NOAA provider", err)
		} else {
			ps[n.GetId()] = n
		}
	}

	router := api.InitServer(ps)
	log.Println("Start server on port 8090")
	log.Fatal(http.ListenAndServe(":8090", router))
}
