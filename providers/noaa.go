package providers

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/a-bouts/wind-server/utils"
	"github.com/go-co-op/gocron"
)

type Noaa struct {
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
	forecasts        map[string]([]string)
	busy             bool
}

func InitNoaaProvider() (Provider, error) {
	p := &Noaa{
		id:             "noaa",
		name:           "NOAA",
		maxForecast:    384,
		step:           3,
		nextUpdateTime: time.Now().Truncate(6 * time.Hour).Add(3 * time.Hour).Add(30 * time.Minute),
		nextRefTime:    time.Now().Truncate(6 * time.Hour),
		gribDir:        "grib-data/noaa/",
		jsonDir:        "json-data/noaa/",
	}

	forecasts, lastRefTime, lastForecastTime, progress, err := parseGribDataFiles(p)
	if err != nil {
		fmt.Println("Error", err)
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

func (p *Noaa) GetId() string {
	return p.id
}

func (p *Noaa) GetName() string {
	return p.name
}

func (p *Noaa) GetLastRefTime() time.Time {
	return p.lastRefTime
}

func (p *Noaa) GetLastForecastTime() int {
	return p.lastForecastTime
}

func (p *Noaa) GetNextUpdateTime() time.Time {
	return p.nextUpdateTime
}

func (p *Noaa) GetNextRefTime() time.Time {
	return p.nextRefTime
}

func (p *Noaa) GetProgress() int {
	return p.progress
}

func (p *Noaa) GetForecasts() map[string]([]string) {
	return p.forecasts
}

func (p *Noaa) getMaxForecastTime() int {
	return p.maxForecast
}

func (p *Noaa) getGribDir() string {
	return p.gribDir
}

func (p *Noaa) GetJsonDir() string {
	return p.jsonDir
}

func (p *Noaa) isBusy() bool {
	return p.busy
}

func (p *Noaa) setBusy(busy bool) {
	p.busy = busy
}

func (p *Noaa) download() {
	download(p)
}

func (p *Noaa) clean() error {
	var toDelete []string

	for s := range p.forecasts {

		t, err := time.Parse("2006010215", s)
		if err != nil {
			log.Println("Noaa :", "Error", err)
			return err
		}

		if t.Before(time.Now().UTC().Add(-3 * time.Hour)) {
			toDelete = append(toDelete, s)
		}
	}

	for _, s := range toDelete {
		for i, file := range p.forecasts[s] {
			log.Println("Noaa :", "Delete", s, p.forecasts[s][i])
			os.Remove(p.gribDir + file)
			os.Remove(p.jsonDir + file)
		}
		delete(p.forecasts, s)
	}

	return nil
}

func (p *Noaa) next(t time.Time) bool {
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
			ok := p.getGribData(t, h)
			if ok {
				forecastFiles, found := p.forecasts[stamp.Key(h)]
				// keep the previous forcast only (no more the following)
				if stamp.FromNow(h) >= 3 && found {
					for _, forecastFile := range forecastFiles {
						log.Println("Noaa :", "Delete", forecastFile)
						os.Remove(p.gribDir + forecastFile)
						os.Remove(p.jsonDir + forecastFile)
					}
					p.forecasts[stamp.Key(h)] = nil
				}
				p.forecasts[stamp.Key(h)] = append(p.forecasts[stamp.Key(h)], stamp.Filename(h))
				downloadedSome = true
			}
			if !ok || h == p.maxForecast {
				// TODO : check mais c'est pas terrible terrible...
				if first {
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

func (p *Noaa) getGribData(moment time.Time, forecast int) bool {
	stamp := utils.StampFromTime(moment)

	ok, err := p.downloadGribData(stamp, forecast)
	if err != nil {
		fmt.Println("Noaa :", "Error downloading grib data", err)
		return false
	}

	if ok {
		err = convertGribToJson(p, stamp, forecast)
		if err != nil {
			fmt.Println("Noaa :", "Error converting grib to json", err)
			return false
		}

		p.lastRefTime = moment
		p.lastForecastTime = forecast
		p.progress = forecast * 100 / p.getMaxForecastTime()

		fmt.Println(p.GetId(), "LastRefTime", p.lastRefTime, "Progress", p.progress)

		return true
	}

	return false
}

func (p *Noaa) downloadGribData(stamp utils.Stamp, forecast int) (ok bool, err error) {
	client := &http.Client{}

	url := "http://nomads.ncep.noaa.gov/cgi-bin/filter_gfs_" + "1p00.pl" + "/gfs." + stamp.Date + "/" + "gfs.t" + stamp.Hour + "z.pgrb2.1p00.f" + fmt.Sprintf("%03d", forecast)
	//url := "https://nomads.ncep.noaa.gov/pub/data/nccf/com/gfs/prod/gfs." + stamp.date + "/" + stamp.hour + "/gfs.t" + stamp.hour + "z.pgrb2.1p00.f" + fmt.Sprintf("%03d", forecast)

	log.Printf("Noaa : Try downloading '%s'", url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Print(err)
		return false, err
	}

	q := req.URL.Query()
	q.Add("file", "gfs.t"+stamp.Hour+"z.pgrb2.1p00.f"+fmt.Sprintf("%03d", forecast))
	q.Add("lev_10_m_above_ground", "on")
	q.Add("lev_surface", "off")
	q.Add("var_TMP", "off")
	q.Add("var_UGRD", "on")
	q.Add("var_VGRD", "on")
	q.Add("var_LAND", "off")
	q.Add("leftlon", "-50")
	q.Add("rightlon", "0")
	q.Add("toplat", "50")
	q.Add("bottomlat", "-30")
	q.Add("dir", "/gfs."+stamp.Date+"/"+stamp.Hour)
	req.URL.RawQuery = q.Encode()

	resp, err := client.Do(req)

	if err != nil {
		fmt.Println("Noaa :", "Error when sending request to the server", err)
		return false, err
	}

	if resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()

		out, err := os.Create(p.gribDir + stamp.Filename(forecast) + ".tmp")
		if err != nil {
			fmt.Println("Noaa :", "Error when sending request to the server", err)
			return false, err
		}

		defer out.Close()

		_, err = io.Copy(out, resp.Body)
		if err != nil {
			fmt.Println("Noaa :", "Error when sending request to the server", err)
			return false, err
		}

		os.Rename(p.gribDir+stamp.Filename(forecast)+".tmp", p.gribDir+stamp.Filename(forecast))

		// gribFile, err := os.Open("grib-data/" + stamp.Filename(forecast) + ".tmp")
		// if err != nil {
		// 	fmt.Printf("\nFile [%s] not found.\n", "grib-data/"+stamp.Filename(forecast)+".tmp")
		// 	return false, err
		// }
		// defer gribFile.Close()
		//
		// reduceFile, err := os.Create("grib-data/" + stamp.Filename(forecast))
		// if err != nil {
		// 	fmt.Printf("Error creating reduced reduceFile: %s", err.Error())
		// 	return false, err
		// }
		//
		// defer reduceFile.Close()
		//
		// end := make(chan bool)
		// content := make(chan []byte)
		//
		// options := griblib.Options{
		// 	Discipline: 0,
		// 	Category:   2,
		// 	Surface:    griblib.Surface{Type: 1}}
		//
		// go griblib.Reduce(gribFile, options, content, end)
		//
		// for {
		// 	select {
		// 	case <-end:
		// 		fmt.Printf("reduce done to file '%s'. \n", "grib-data/"+stamp.Filename(forecast))
		// 		log.Println(resp.StatusCode, ": Downloading OK")
		// 		return true, nil
		// 	case bytesRead := <-content:
		// 		reduceFile.Write(bytesRead)
		// 	}
		// }

		log.Println("Noaa :", resp.StatusCode, ": Downloading OK")
		return true, nil
	}

	log.Println("Noaa :", resp.StatusCode, ": Downloading KO")
	return false, nil
}
