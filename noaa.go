package main

import (
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jasonlvhit/gocron"
)

type Noaa struct {
	Forecasts      map[string]string
	RefreshWebhook string
}

type locker struct {
	locked bool
}

func (s *locker) Lock(key string) (success bool, err error) {
	if s.locked {
		return false, nil
	}
	s.locked = true
	return true, nil
}

func (s *locker) Unlock(key string) error {
	s.locked = false
	return nil
}

func InitNoaa(refreshWebhook string) (*Noaa, error) {
	forecasts, err := parseGribDataFiles()
	if err != nil {
		fmt.Println("Error", err)
		return nil, err
	}

	n := &Noaa{
		Forecasts:      forecasts,
		RefreshWebhook: refreshWebhook}

	from := time.Now().Truncate(time.Minute * 5).Add(time.Minute * 5)
	s := gocron.NewScheduler()
	gocron.SetLocker(&locker{})
	s.Every(5).Minutes().From(&from).Lock().Do(n.download)
	go s.Start()

	return n, nil
}

func (n *Noaa) download() {
	log.Println("Something to delete ?")
	n.clean()
	log.Println("Something to download ?")
	if n.nextToDownload(time.Now()) {
		_, err := http.Get(n.RefreshWebhook)
		if err != nil {
			log.Println("Error calling refresh webhook", err)
		}
	}
}

func (n *Noaa) clean() error {
	var toDelete []string

	for s := range n.Forecasts {

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
		log.Println("Delete", s, n.Forecasts[s])
		os.Remove("grib-data/" + n.Forecasts[s])
		os.Remove("json-data/" + n.Forecasts[s])
		delete(n.Forecasts, s)
	}

	return nil
}

func (n *Noaa) nextToDownload(t time.Time) bool {
	h := 0

	downloadedSome := false

	p := t.Add(time.Hour * time.Duration(-1*(t.UTC().Hour()%6)))

	if time.Now().Sub(p) < time.Hour*3 {
		return n.nextToDownload(t.Add(-6 * time.Hour))
	}

	stamp := stampFromTime(t)

	for h <= 384 {
		_, found := n.Forecasts[stamp.key(h)]
		if stamp.fromNow(h) <= -3 || stamp.fromNow(h) <= 3 && found {
			h += 3
			continue
		}
		if _, err := os.Stat("grib-data/" + stamp.filename(h)); os.IsNotExist(err) {
			ok := n.getGribData(t, h)
			if ok {
				forecastFile, found := n.Forecasts[stamp.key(h)]
				if stamp.fromNow(h) >= 0 && found {
					log.Println("Delete", forecastFile)
					os.Remove("grib-data/" + forecastFile)
					os.Remove("json-data/" + forecastFile)
				}
				n.Forecasts[stamp.key(h)] = stamp.filename(h)
				downloadedSome = true
			}
			if !ok || h == 384 {
				// TODO : check mais c'est pas terrible terrible...
				if h <= 3 {
					return n.nextToDownload(t.Add(-6 * time.Hour))
				}
				//on arrete là pour le moment
				break
			}
		}
		h += 3
	}

	return downloadedSome
}

func parseGribDataFiles() (map[string]string, error) {
	var files []string
	err := filepath.Walk("grib-data", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Println(err)
		} else if info.Mode().IsRegular() {
			files = append(files, info.Name())
		}
		return nil
	})
	if err != nil {
		fmt.Println("Error", err)
		return nil, err
	}

	sort.Strings(files)

	forecasts := make(map[string]string)

	for cpt, f := range files {

		fmt.Println(f)

		stamp := stampFromFile(f)

		h, err := strconv.Atoi(strings.Split(f, ".")[1][1:])
		if err != nil {
			fmt.Println("Error", err)
			return nil, err
		}

		forecastHour := stamp.fromNow(h)

		if forecastHour < -3 && cpt < len(files)-1 {
			log.Println("Delete", f)
			os.Remove("grib-data/" + f)
			os.Remove("json-data/" + f)
			continue
		}

		forecastFile, found := forecasts[stamp.key(h)]
		if forecastHour >= 0 && found {
			log.Println("Delete", forecastFile)
			os.Remove("grib-data/" + forecastFile)
			os.Remove("json-data/" + forecastFile)
		}

		//quand c'est la prévision courante, on la conserve meme si une nouvelle prévision est arrivé
		if !found || forecastHour >= 3 {
			forecasts[stamp.key(h)] = f
		}
	}

	return forecasts, nil
}

func (n *Noaa) getGribData(moment time.Time, forecast int) bool {
	stamp := stampFromTime(moment)

	ok, err := downloadGribData(stamp, forecast)
	if err != nil {
		fmt.Println("Error downloading grib data", err)
		return false
	}

	if ok {
		err = convertGribToJson(stamp, forecast)
		if err != nil {
			fmt.Println("Error converting grib to json", err)
			return false
		}

		return true
	}

	return false
}

func downloadGribData(stamp Stamp, forecast int) (ok bool, err error) {
	client := &http.Client{}

	url := "http://nomads.ncep.noaa.gov/cgi-bin/filter_gfs_" + "1p00.pl" + "/gfs." + stamp.date + "/" + "gfs.t" + stamp.hour + "z.pgrb2.1p00.f" + fmt.Sprintf("%03d", forecast)

	log.Printf("Try downloading '%s'", url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Print(err)
		return false, err
	}

	q := req.URL.Query()
	q.Add("file", "gfs.t"+stamp.hour+"z.pgrb2.1p00.f"+fmt.Sprintf("%03d", forecast))
	q.Add("lev_10_m_above_ground", "on")
	q.Add("lev_surface", "on")
	q.Add("var_TMP", "on")
	q.Add("var_UGRD", "on")
	q.Add("var_VGRD", "on")
	q.Add("var_LAND", "on")
	q.Add("leftlon", "-50")
	q.Add("rightlon", "0")
	q.Add("toplat", "50")
	q.Add("bottomlat", "-30")
	q.Add("dir", "/gfs."+stamp.date+"/"+stamp.hour)
	req.URL.RawQuery = q.Encode()

	resp, err := client.Do(req)

	if err != nil {
		fmt.Println("Error when sending request to the server", err)
		return false, err
	}

	if resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()

		out, err := os.Create("grib-data/" + stamp.filename(forecast))
		if err != nil {
			fmt.Println("Error when sending request to the server", err)
			return false, err
		}

		defer out.Close()

		_, err = io.Copy(out, resp.Body)
		if err != nil {
			fmt.Println("Error when sending request to the server", err)
			return false, err
		}

		log.Println(resp.StatusCode, ": Downloading OK")
		return true, nil
	}

	log.Println(resp.StatusCode, ": Downloading KO")
	return false, nil
}

func convertGribToJson(stamp Stamp, forecast int) error {

	lw := log.Writer()

	args := []string{
		"--data",
		"--output",
		"json-data/" + stamp.filename(forecast),
		"--names",
		"--compact",
		"grib-data/" + stamp.filename(forecast)}

	cmd := exec.Command("grib2json/bin/grib2json", args...)
	cmd.Env = append(os.Environ())
	cmd.Stdout = lw
	cmd.Stderr = lw
	return cmd.Run()
}

type Stamp struct {
	date string
	hour string
}

func (s *Stamp) fromNow(h int) int {
	t, err := time.Parse("2006010215", s.date+s.hour)
	if err != nil {
		log.Fatalln("Error", err)
		return 0
	}

	t = t.Add(time.Hour * time.Duration(h))

	forecastHour := int(math.Round(t.Sub(time.Now()).Hours()))

	return forecastHour
}

func stampFromTime(t time.Time) Stamp {
	p := t.Add(time.Hour * time.Duration(-1*(t.UTC().Hour()%6)))

	return Stamp{date: p.UTC().Format("20060102"), hour: p.UTC().Format("15")}
}

func stampFromFile(f string) Stamp {
	d := strings.Split(f, ".")[0]

	return Stamp{date: d[0 : len(d)-2], hour: d[len(d)-2:]}
}

func (s *Stamp) filename(h int) string {
	return s.date + s.hour + ".f" + fmt.Sprintf("%03d", h)
}

func (s *Stamp) key(h int) string {
	t, err := time.Parse("2006010215", s.date+s.hour)
	if err != nil {
		log.Fatalln("Error", err)
		return ""
	}

	t = t.Add(time.Hour * time.Duration(h))

	return t.Format("2006010215")
}
