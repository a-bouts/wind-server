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
	"time"

	"github.com/a-bouts/wind-server/utils"
	"github.com/gorilla/mux"
)

func (s server) setApiV2(router *mux.Router) {
	apiV2 := router.PathPrefix("/winds/api/v2").Subrouter()
	apiV2.HandleFunc("/providers", s.getProvidersHandler).Methods(http.MethodGet)
	apiV2.HandleFunc("/providers/{provider}/winds", s.getWindsHandler).Methods(http.MethodGet)
	apiV2.HandleFunc("/providers/{provider}/winds/{forecast}/{refTime}", s.getWindHandler).Methods(http.MethodGet)
}

func (s server) getProvidersHandler(w http.ResponseWriter, r *http.Request) {

	providers := make([]Provider, 0, len(s.providers))
	for k, v := range s.providers {
		providers = append(providers, Provider{
			Id:   k,
			Name: v.GetName(),
		})
	}

	if err := json.NewEncoder(w).Encode(providers); err != nil {
		log.Println("Failed to serialize providers list :", providers, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s server) getWindsHandler(w http.ResponseWriter, r *http.Request) {

	providerId := mux.Vars(r)["provider"]

	provider, found := s.providers[providerId]
	if !found {
		http.Error(w, fmt.Sprintf("%s provider do not exists", providerId), http.StatusNotFound)
		return
	}

	winds := Winds{
		Provider:         provider.GetId(),
		LastRefTime:      provider.GetLastRefTime().UTC(),
		LastForecastTime: provider.GetLastForecastTime(),
		NextUpdateTime:   provider.GetNextUpdateTime().UTC(),
		NextRefTime:      provider.GetNextRefTime().UTC(),
		Progress:         provider.GetProgress(),
	}

	var keys []string
	for k := range provider.GetForecasts() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		stamp := utils.StampFromFile(provider.GetForecasts()[k][0])
		h, err := strconv.Atoi(strings.Split(provider.GetForecasts()[k][0], ".")[1][1:])
		if err != nil {
			fmt.Println("Error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		f, err := time.Parse("2006010215", k)
		if err != nil {
			log.Println("Error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		r, err := time.Parse("2006010215", stamp.Date+stamp.Hour)
		if err != nil {
			log.Println("Error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		stamps := []Stamp{Stamp{
			RefTime:      r,
			ForecastTime: h,
		}}

		forecast := Forecast{
			Forecast: f,
			Stamps:   stamps,
		}

		if len(provider.GetForecasts()[k]) > 1 {
			stamp2 := utils.StampFromFile(provider.GetForecasts()[k][1])

			r, err = time.Parse("2006010215", stamp2.Date+stamp2.Hour)
			if err != nil {
				log.Println("Error", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			h, err = strconv.Atoi(strings.Split(provider.GetForecasts()[k][1], ".")[1][1:])
			if err != nil {
				fmt.Println("Error", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			forecast.Stamps = append(forecast.Stamps, Stamp{
				RefTime:      r,
				ForecastTime: h,
			})
		}

		winds.LastForecast = f
		winds.Forecasts = append(winds.Forecasts, forecast)
	}

	if err := json.NewEncoder(w).Encode(winds); err != nil {
		log.Println("Failed to serialize forecasts :", provider.GetForecasts(), err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s server) getWindHandler(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Cache-Control", "public, max-age=10800, immutable")

	providerId := mux.Vars(r)["provider"]
	forecast := mux.Vars(r)["forecast"]
	refTime := mux.Vars(r)["refTime"]

	log.Println(providerId, forecast, refTime)

	provider, found := s.providers[providerId]
	if !found {
		http.Error(w, fmt.Sprintf("%s provider do not exists", providerId), http.StatusNotFound)
		return
	}

	if _, found := provider.GetForecasts()[forecast]; !found {
		http.Error(w, fmt.Sprintf("%s forecast do not exist for provider %s", forecast, providerId), http.StatusNotFound)
		return
	}

	for _, f := range provider.GetForecasts()[forecast] {
		if refTime == strings.Split(f, ".")[0] {
			in, err := os.Open(provider.GetJsonDir() + f)
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

	http.Error(w, fmt.Sprintf("%s refTime do not exist for provider %s forecast %s", refTime, providerId, forecast), http.StatusNotFound)
}
