package api

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

	"github.com/a-bouts/wind-server/providers"
	"github.com/a-bouts/wind-server/utils"
	"github.com/gorilla/mux"
)

type server struct {
	providers map[string]providers.Provider
}

func InitServer(providers map[string]providers.Provider) *mux.Router {

	router := mux.NewRouter().StrictSlash(true)

	s := server{providers: providers}

	router.HandleFunc("/winds/-/healthz", s.healthz).Methods(http.MethodGet)

	router.HandleFunc("/winds", s.oldGetWindsHandler).Methods(http.MethodGet)
	router.HandleFunc("/winds/{forecast}", s.oldGetWindHandler).Methods(http.MethodGet)
	router.HandleFunc("/winds/{forecast}/{stamp}", s.oldGetWindHandlerByStampOld).Methods(http.MethodGet)

	apiV1 := router.PathPrefix("/winds/api/v1").Subrouter()
	apiV1.HandleFunc("/", s.oldGetWindsHandler).Methods(http.MethodPost)
	apiV1.HandleFunc("/winds/{forecast}/{stamp}", s.oldGetWindHandlerByStamp).Methods(http.MethodGet)

	s.setApiV2(router)

	return router
}

func (s server) healthz(w http.ResponseWriter, req *http.Request) {
	type health struct {
		Status string `json:"status"`
	}

	json.NewEncoder(w).Encode(health{Status: "Ok"})
}

type OldForecast struct {
	Hour     int    `json:"hour"`
	Stamp    string `json:"stamp"`
	Stamp2   string `json:"stamp2"`
	Forecast string `json:"forecast"`
}

func (s server) oldGetWindsHandler(w http.ResponseWriter, r *http.Request) {

	var forecasts []OldForecast

	var keys []string
	for k := range s.providers["noaa"].GetForecasts() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		stamp := utils.StampFromFile(s.providers["noaa"].GetForecasts()[k][0])
		h, err := strconv.Atoi(strings.Split(s.providers["noaa"].GetForecasts()[k][0], ".")[1][1:])
		if err != nil {
			fmt.Println("Error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		forecast := OldForecast{Hour: stamp.FromNow(h), Stamp: stamp.Date + stamp.Hour, Forecast: k}

		if len(s.providers["noaa"].GetForecasts()[k]) > 1 {
			stamp2 := utils.StampFromFile(s.providers["noaa"].GetForecasts()[k][1])
			forecast.Stamp2 = stamp2.Date + stamp2.Hour
		}

		forecasts = append(forecasts, forecast)
	}

	if err := json.NewEncoder(w).Encode(forecasts); err != nil {
		log.Println("Failed to serialize forecasts :", s.providers["noaa"].GetForecasts(), err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s server) oldGetWindHandlerByStampOld(w http.ResponseWriter, r *http.Request) {

	forecast := mux.Vars(r)["forecast"]
	stamp := mux.Vars(r)["stamp"]

	log.Println(forecast, stamp)

	if _, found := s.providers["noaa"].GetForecasts()[forecast]; !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	i, err := strconv.Atoi(stamp)
	if err != nil {
		fmt.Println("Error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	in, err := os.Open("json-data/noaa/" + s.providers["noaa"].GetForecasts()[forecast][i])
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

func (s server) oldGetWindHandlerByStamp(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Cache-Control", "public, max-age=10800, immutable")

	forecast := mux.Vars(r)["forecast"]
	stamp := mux.Vars(r)["stamp"]

	log.Println(forecast, stamp)

	if _, found := s.providers["noaa"].GetForecasts()[forecast]; !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	for _, f := range s.providers["noaa"].GetForecasts()[forecast] {
		if stamp == strings.Split(f, ".")[0] {
			in, err := os.Open("json-data/noaa/" + f)
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

func (s server) oldGetWindHandler(w http.ResponseWriter, r *http.Request) {

	forecast := mux.Vars(r)["forecast"]

	log.Println(forecast)

	if _, found := s.providers["noaa"].GetForecasts()[forecast]; !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	in, err := os.Open("json-data/noaa/" + s.providers["noaa"].GetForecasts()[forecast][0])
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
