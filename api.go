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

	router.HandleFunc("/winds/-/healthz", s.healthz).Methods(http.MethodGet)
	router.HandleFunc("/winds", s.getWindsHandler).Methods(http.MethodGet)
	router.HandleFunc("/winds/{forecast}", s.getWindHandler).Methods(http.MethodGet)
	router.HandleFunc("/winds/{forecast}/{stamp}", s.getWindHandlerByStampOld).Methods(http.MethodGet)

	apiV1 := router.PathPrefix("/winds/api/v1").Subrouter()
	apiV1.HandleFunc("/winds", s.getWindsHandler).Methods(http.MethodGet)
	apiV1.HandleFunc("/winds/{forecast}/{stamp}", s.getWindHandlerByStamp).Methods(http.MethodGet)

	return router
}

func (s server) healthz(w http.ResponseWriter, req *http.Request) {
	type health struct {
		Status string `json:"status"`
	}

	json.NewEncoder(w).Encode(health{Status: "Ok"})
}

type Forecast struct {
	Hour     int    `json:"hour"`
	Stamp    string `json:"stamp"`
	Stamp2   string `json:"stamp2"`
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
		stamp := stampFromFile(s.n.Forecasts[k][0])
		h, err := strconv.Atoi(strings.Split(s.n.Forecasts[k][0], ".")[1][1:])
		if err != nil {
			fmt.Println("Error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		forecast := Forecast{Hour: stamp.fromNow(h), Stamp: stamp.date + stamp.hour, Forecast: k}

		if len(s.n.Forecasts[k]) > 1 {
			stamp2 := stampFromFile(s.n.Forecasts[k][1])
			forecast.Stamp2 = stamp2.date + stamp2.hour
		}

		forecasts = append(forecasts, forecast)
	}

	if err := json.NewEncoder(w).Encode(forecasts); err != nil {
		log.Println("Failed to serialize forecasts :", s.n.Forecasts, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s server) getWindHandlerByStampOld(w http.ResponseWriter, r *http.Request) {

	forecast := mux.Vars(r)["forecast"]
	stamp := mux.Vars(r)["stamp"]

	log.Println(forecast, stamp)

	if _, found := s.n.Forecasts[forecast]; !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	i, err := strconv.Atoi(stamp)
	if err != nil {
		fmt.Println("Error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	in, err := os.Open("json-data/" + s.n.Forecasts[forecast][i])
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

func (s server) getWindHandlerByStamp(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Cache-Control", "public, max-age=10800, immutable")

	forecast := mux.Vars(r)["forecast"]
	stamp := mux.Vars(r)["stamp"]

	log.Println(forecast, stamp)

	if _, found := s.n.Forecasts[forecast]; !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	for _, f := range s.n.Forecasts[forecast] {
		if stamp == strings.Split(f, ".")[0] {
			in, err := os.Open("json-data/" + f)
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

			return
		}
	}

	w.WriteHeader(http.StatusNotFound)
}

func (s server) getWindHandler(w http.ResponseWriter, r *http.Request) {

	forecast := mux.Vars(r)["forecast"]

	log.Println(forecast)

	if _, found := s.n.Forecasts[forecast]; !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	in, err := os.Open("json-data/" + s.n.Forecasts[forecast][0])
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
