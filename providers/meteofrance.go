package providers

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/a-bouts/wind-server/utils"
	"github.com/go-co-op/gocron"
)

type MeteoFrance struct {
	id               string
	name             string
	lastRefTime      time.Time
	lastForecastTime int
	nextUpdateTime   time.Time
	nextRefTime      time.Time
	progress         int
	maxForecast      int
	step             int
	gribDir          string
	jsonDir          string
	token            string
	forecasts        map[string]([]string)
	busy             bool
}

func InitMeteoFranceProvider(token string) (Provider, error) {
	p := &MeteoFrance{
		id:             "meteo-france",
		name:           "Meteo-France",
		maxForecast:    104,
		step:           3,
		nextUpdateTime: time.Now().Truncate(6 * time.Hour).Add(3 * time.Hour).Add(30 * time.Minute),
		nextRefTime:    time.Now().Truncate(6 * time.Hour),
		gribDir:        "grib-data/meteo-france/",
		jsonDir:        "json-data/meteo-france/",
		token:          token,
	}

	now := time.Now().Truncate(6 * time.Hour)
	p.init(now)

	forecasts, lastRefTime, lastForecastTime, progress, err := parseGribDataFiles(p)
	if err != nil {
		fmt.Println("Meteo-France :", "Error", err)
		return nil, err
	}
	p.forecasts = forecasts
	p.lastRefTime = lastRefTime
	p.lastForecastTime = lastForecastTime
	p.progress = progress

	from := time.Now().Truncate(time.Minute * 5).Add(time.Minute * 5)
	s := gocron.NewScheduler(time.UTC)
	gocron.SetLocker(&locker{})
	s.Every(5).Minutes().StartAt(from.UTC()).Lock().Do(p.download)
	go s.StartBlocking()

	// p.download()

	return p, nil
}

func (p *MeteoFrance) download() {
	download(p)
}

func (p *MeteoFrance) GetId() string {
	return p.id
}

func (p *MeteoFrance) GetName() string {
	return p.name
}

func (p *MeteoFrance) GetLastRefTime() time.Time {
	return p.lastRefTime
}

func (p *MeteoFrance) GetLastForecastTime() int {
	return p.lastForecastTime
}

func (p *MeteoFrance) GetNextUpdateTime() time.Time {
	return p.nextUpdateTime
}

func (p *MeteoFrance) GetNextRefTime() time.Time {
	return p.nextRefTime
}

func (p *MeteoFrance) GetProgress() int {
	return p.progress
}

func (p *MeteoFrance) GetForecasts() map[string]([]string) {
	return p.forecasts
}

func (p *MeteoFrance) getMaxForecastTime() int {
	return p.maxForecast
}

func (p *MeteoFrance) getGribDir() string {
	return p.gribDir
}

func (p *MeteoFrance) GetJsonDir() string {
	return p.jsonDir
}

func (p *MeteoFrance) isBusy() bool {
	return p.busy
}

func (p *MeteoFrance) setBusy(busy bool) {
	p.busy = busy
}

func (p *MeteoFrance) clean() error {
	var toDelete []string

	for s := range p.forecasts {

		t, err := time.Parse("2006010215", s)
		if err != nil {
			log.Println("Error", err)
			return err
		}

		if t.Before(time.Now().UTC().Add(-3 * time.Hour)) {
			toDelete = append(toDelete, s)
		}
	}

	for _, s := range toDelete {
		for i, file := range p.forecasts[s] {
			log.Println("Meteo-France :", "Delete", s, p.forecasts[s][i])
			os.Remove(p.gribDir + file)
			os.Remove(p.jsonDir + file)
		}
		delete(p.forecasts, s)
	}

	return nil
}

type CoverageDescriptions struct {
	CoverageDescription CoverageDescription `xml:"CoverageDescription"`
}

// type CoverageDescriptions []CoverageDescription

type CoverageDescription struct {
	CoverageId string    `xml:"CoverageId"`
	DomainSet  DomainSet `xml:"domainSet"`
}

type DomainSet struct {
	ReferenceableGridByVectors ReferenceableGridByVectors `xml:"ReferenceableGridByVectors"`
}

type ReferenceableGridByVectors struct {
	GeneralGridAxis []GeneralGridAxis `xml:"generalGridAxis"`
}

type GeneralGridAxis struct {
	GridAxesSpanned string `xml:"GeneralGridAxis>gridAxesSpanned"`
	Coefficients    string `xml:"GeneralGridAxis>coefficients"`
}

func (p *MeteoFrance) init(refTime time.Time) {
	maxForecastTime, err := p.describeCoverage(refTime)
	time.Sleep(5 * time.Second)
	if err != nil {
		p.init(refTime.Add(-6 * time.Hour))
	} else {
		p.maxForecast = maxForecastTime
		fmt.Println(p.GetId(), "max forecast", fmt.Sprintf("%2dZ", refTime.UTC().Hour()), p.maxForecast)
	}
}

func (p *MeteoFrance) describeCoverage(refTime time.Time) (int, error) {
	client := &http.Client{}

	uri := "https://geoservices.meteofrance.fr/api/" + p.token + "/MF-NWP-GLOBAL-ARPEGE-025-GLOBE-WCS"

	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		log.Print(err)
		return -1, err
	}

	q := req.URL.Query()
	q.Add("service", "WCS")
	q.Add("version", "2.0.1")
	q.Add("request", "DescribeCoverage")
	q.Add("coverageid", "U_COMPONENT_OF_WIND__SPECIFIC_HEIGHT_LEVEL_ABOVE_GROUND___"+refTime.UTC().Format("2006-01-02T15")+".00.00Z")
	req.URL.RawQuery = q.Encode()

	qu, _ := url.QueryUnescape(req.URL.RequestURI())

	log.Printf("Meteo-France : Try downloading '%s'", req.URL.Scheme+"://"+req.URL.Host+qu)

	resp, err := client.Do(req)

	if err != nil {
		fmt.Println("Meteo-France :", "Error when sending request to the server", err)
		return -1, err
	}

	if resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Println(p.GetId(), "Error reading Describe Body", err)
			return -1, err
		}

		var desc CoverageDescriptions

		xml.Unmarshal(body, &desc)

		for _, g := range desc.CoverageDescription.DomainSet.ReferenceableGridByVectors.GeneralGridAxis {
			if g.GridAxesSpanned == "time" {
				times := strings.Split(g.Coefficients, " ")
				time, err := strconv.Atoi(times[len(times)-1])
				if err != nil {
					fmt.Println(p.GetId(), "Error getting last time from describe", err)
					return -1, err
				}

				return time / 3600, nil
			}
		}
		log.Println(p.GetId(), resp.StatusCode, ": Describe OK", desc.CoverageDescription.CoverageId)
		return 104, nil // default value if time not found
	}

	b := ""
	if resp.StatusCode != http.StatusNotFound {
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading body: %v", err)
		}
		b = string(body)
	}

	log.Println("Meteo-France :", resp.StatusCode, ": Describe KO", b)
	return -1, errors.New("Describe Coverage not found")
}

func (p *MeteoFrance) next(t time.Time) bool {

	h := 0
	first := true

	downloadedSome := false

	stamp := utils.StampFromTime(t)

	for h <= p.maxForecast {
		_, found := p.forecasts[stamp.Key(h)]
		if stamp.FromNow(h) <= -3 || stamp.FromNow(h) <= 0 && found {
			h += p.step
			continue
		}
		if _, err := os.Stat(p.gribDir + stamp.Filename(h)); os.IsNotExist(err) {
			if first {
				maxForecastTime, err := p.describeCoverage(t)
				time.Sleep(5 * time.Second)
				if err != nil {
					return p.next(t.Add(-6 * time.Hour))
				} else {
					p.maxForecast = maxForecastTime
				}
			}

			ok := p.getGribData(t, h)
			if ok {
				forecastFiles, found := p.forecasts[stamp.Key(h)]
				// keep the previous forcast only (no more the following)
				if stamp.FromNow(h) >= 3 && found {
					for _, forecastFile := range forecastFiles {
						log.Println("Meteo-France :", "Delete", forecastFile)
						os.Remove(p.gribDir + forecastFile)
						os.Remove(p.jsonDir + forecastFile)
					}
					p.forecasts[stamp.Key(h)] = nil
				}
				p.forecasts[stamp.Key(h)] = append(p.forecasts[stamp.Key(h)], stamp.Filename(h))
				downloadedSome = true
			}
			if !ok || h == 384 {
				// TODO : check mais c'est pas terrible terrible...
				if first {
					time.Sleep(5 * time.Second)
					return p.next(t.Add(-6 * time.Hour))
				}
				//on arrete là pour le moment
				break
			}
		}
		h += p.step
		first = false
	}

	return downloadedSome
}

func (p *MeteoFrance) getGribData(ref time.Time, forecast int) bool {
	stamp := utils.StampFromTime(ref)

	ok, err := p.downloadGribData(ref, forecast)
	if err != nil {
		fmt.Println("Meteo-France :", "Error downloading grib data", err)
		return false
	}

	if ok {
		err = convertGribToJson(p, stamp, forecast)
		if err != nil {
			fmt.Println("Meteo-France :", "Error converting grib to json", err)
			return false
		}

		p.lastRefTime = ref
		p.lastForecastTime = forecast
		p.progress = forecast * 100 / p.getMaxForecastTime()

		fmt.Println(p.GetId(), "LastRefTime", p.lastRefTime, "Progress", p.progress)

		return true
	}

	return false
}

func (p *MeteoFrance) downloadGribData(ref time.Time, forecast int) (bool, error) {

	u, err := p.downloadGribDataComponent(ref, forecast, "U")
	if err != nil {
		return false, err
	}
	if u {
		time.Sleep(5 * time.Second)
		v, err := p.downloadGribDataComponent(ref, forecast, "V")
		if err != nil {
			return false, err
		}
		if v {
			time.Sleep(5 * time.Second)
			// merge les deux fichiers
			stamp := utils.StampFromTime(ref)

			lw := log.Writer()

			args := []string{
				p.gribDir + "U_" + stamp.Filename(forecast) + ".tmp",
				p.gribDir + "V_" + stamp.Filename(forecast) + ".tmp",
				p.gribDir + stamp.Filename(forecast) + ".tmp"}

			cmd := exec.Command("grib_copy", args...)
			cmd.Env = append(os.Environ())
			cmd.Stdout = lw
			cmd.Stderr = lw

			err = cmd.Run()
			if err != nil {
				return false, err
			}

			os.Remove(p.gribDir + "U_" + stamp.Filename(forecast) + ".tmp")
			os.Remove(p.gribDir + "V_" + stamp.Filename(forecast) + ".tmp")

			os.Rename(p.gribDir+stamp.Filename(forecast)+".tmp", p.gribDir+stamp.Filename(forecast))

			return true, nil
		}
	}

	return false, nil
}

func (p *MeteoFrance) downloadGribDataComponent(ref time.Time, forecast int, component string) (bool, error) {
	client := &http.Client{}

	uri := "https://geoservices.meteofrance.fr/api/" + p.token + "/MF-NWP-GLOBAL-ARPEGE-025-GLOBE-WCS"

	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		log.Print(err)
		return false, err
	}

	q := req.URL.Query()
	q.Add("service", "WCS")
	q.Add("version", "2.0.1")
	q.Add("request", "GetCoverage")
	q.Add("format", "application/wmo-grib")
	q.Add("coverageid", component+"_COMPONENT_OF_WIND__SPECIFIC_HEIGHT_LEVEL_ABOVE_GROUND___"+ref.UTC().Format("2006-01-02T15")+".00.00Z")
	q.Add("subset", "height(10)")
	q.Add("subset", "time("+ref.Add(time.Hour*time.Duration(forecast)).UTC().Format("2006-01-02T15")+":00:00Z)")
	req.URL.RawQuery = q.Encode()

	qu, _ := url.QueryUnescape(req.URL.RequestURI())

	log.Printf("Meteo-France : Try downloading '%s'", req.URL.Scheme+"://"+req.URL.Host+qu)

	resp, err := client.Do(req)

	if err != nil {
		fmt.Println("Meteo-France :", "Error when sending request to the server", err)
		return false, err
	}

	if resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()

		stamp := utils.StampFromTime(ref)

		out, err := os.Create(p.gribDir + component + "_" + stamp.Filename(forecast) + ".tmp")
		if err != nil {
			fmt.Println("Meteo-France :", "Error when sending request to the server", err)
			return false, err
		}

		defer out.Close()

		_, err = io.Copy(out, resp.Body)
		if err != nil {
			fmt.Println("Meteo-France :", "Error when sending request to the server", err)
			return false, err
		}

		log.Println("Meteo-France :", resp.StatusCode, ": Downloading OK")
		return true, nil
	}

	b := ""
	if resp.StatusCode != http.StatusNotFound {
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading body: %v", err)
		}
		b = string(body)
	}

	log.Println("Meteo-France :", resp.StatusCode, ": Downloading KO", b)
	return false, nil
}
