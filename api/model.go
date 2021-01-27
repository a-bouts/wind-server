package api

import "time"

type Provider struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type Winds struct {
	Provider         string     `json:"provider"`
	LastRefTime      time.Time  `json:"lastRefTime"`
	LastForecastTime int        `json:"lastForecastTime"`
	NextUpdateTime   time.Time  `json:"nextUpdateTime"`
	NextRefTime      time.Time  `json:"nextRefTime"`
	Progress         int        `json:"progress"`
	LastForecast     time.Time  `json:"lastForecast"`
	Forecasts        []Forecast `json:"forecasts"`
}

type Forecast struct {
	Forecast time.Time `json:"forecast"`
	Stamps   []Stamp   `json:"stamps"`
}

type Stamp struct {
	RefTime      time.Time `json:"refTime"`
	ForecastTime int       `json:"forecastTime"`
}
