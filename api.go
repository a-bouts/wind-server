package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
)

type server struct {
	n *Noaa
}

func InitServer(n *Noaa) *mux.Router {

	router := mux.NewRouter().StrictSlash(true)

	s := server{n: n}

	router.HandleFunc("/winds", s.getWindsHandler).Methods(http.MethodGet)
	router.HandleFunc("/winds/{forecast}", s.getWindHandler).Methods(http.MethodGet)

	return router
}

type Forecast struct {
	Hour     int    `json:"hour"`
	Stamp    string `json:"stamp"`
	Forecast string `json:"forecast"`
}

func (s server) getWindsHandler(w http.ResponseWriter, r *http.Request) {

	var forecasts []Forecast

	var keys []string
	for k := range s.n.Forecasts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		stamp := stampFromFile(s.n.Forecasts[k])
		h, err := strconv.Atoi(strings.Split(s.n.Forecasts[k], ".")[1][1:])
		if err != nil {
			fmt.Println("Error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		forecast := Forecast{Hour: stamp.fromNow(h), Stamp: stamp.date + stamp.hour, Forecast: k}
		forecasts = append(forecasts, forecast)
	}

	if err := json.NewEncoder(w).Encode(forecasts); err != nil {
		log.Println("Failed to serialize forecasts :", s.n.Forecasts, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s server) getWindHandler(w http.ResponseWriter, r *http.Request) {

	forecast := mux.Vars(r)["forecast"]

	log.Println(forecast)

	if _, found := s.n.Forecasts[forecast]; !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	in, err := os.Open("json-data/" + s.n.Forecasts[forecast])
	if err != nil {
		fmt.Println("Error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	defer in.Close()

	_, err = io.Copy(w, in)
	if err != nil {
		fmt.Println("Error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
