package providers

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/a-bouts/wind-server/utils"
)

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

type Provider interface {
	GetId() string
	GetName() string
	GetLastRefTime() time.Time
	GetLastForecastTime() int
	GetNextUpdateTime() time.Time
	GetNextRefTime() time.Time
	GetProgress() int
	GetForecasts() map[string]([]string)
	getMaxForecastTime() int
	getGribDir() string
	GetJsonDir() string
	isBusy() bool
	setBusy(bool)
	next(time.Time) bool
	clean() error
}

func download(p Provider) {
	if p.isBusy() {
		return
	}
	p.setBusy(true)
	log.Println(p.GetId(), "Something to delete ?")
	p.clean()
	log.Println(p.GetId(), "Something to download ?")

	now := time.Now().Truncate(6 * time.Hour)
	p.next(now)
	p.setBusy(false)
}

func parseGribDataFiles(p Provider) (map[string][]string, time.Time, int, int, error) {
	forecasts := make(map[string][]string)
	var lastRefTime time.Time
	lastForecastTime := -1
	progress := 0

	var files []string
	err := filepath.Walk(p.getGribDir(), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Println(err)
		} else if info.Mode().IsRegular() && !strings.HasSuffix(info.Name(), ".tmp") {
			files = append(files, info.Name())
		}
		return nil
	})
	if err != nil {
		fmt.Println("Error", err)
		return nil, lastRefTime, lastForecastTime, progress, err
	}

	sort.Strings(files)

	for cpt, f := range files {

		fmt.Println(f)

		stamp := utils.StampFromFile(f)

		h, err := strconv.Atoi(strings.Split(f, ".")[1][1:])
		if err != nil {
			fmt.Println("Error", err)
			return nil, lastRefTime, lastForecastTime, progress, err
		}

		forecastHour := stamp.FromNow(h)

		if forecastHour < -3 && cpt < len(files)-1 {
			log.Println("Delete", f)
			os.Remove(p.getGribDir() + f)
			os.Remove(p.GetJsonDir() + f)
			continue
		}

		forecastFiles, found := forecasts[stamp.Key(h)]
		if forecastHour >= 3 && found {
			for _, forecastFile := range forecastFiles {
				log.Println("Delete", forecastFile)
				os.Remove(p.getGribDir() + forecastFile)
				os.Remove(p.GetJsonDir() + forecastFile)
			}
		}

		lastRefTime, _ = time.Parse("2006010215", stamp.Date+stamp.Hour)
		lastForecastTime = h
		progress = h * 100 / p.getMaxForecastTime()

		//quand c'est la prévision courante, on la conserve meme si une nouvelle prévision est arrivé
		if !found || forecastHour >= 0 {
			forecasts[stamp.Key(h)] = append(forecasts[stamp.Key(h)], f)
			fmt.Println(stamp.Key(h), forecasts[stamp.Key(h)])
		}
	}

	fmt.Println(p.GetId(), "LastRefTime", lastRefTime, "Progress", progress)
	return forecasts, lastRefTime, lastForecastTime, progress, nil
}

func convertGribToJson(p Provider, stamp utils.Stamp, forecast int) error {

	lw := log.Writer()

	args := []string{
		"--data",
		"--output",
		p.GetJsonDir() + stamp.Filename(forecast),
		"--names",
		"--fs",
		"103",
		"--fv",
		"10",
		"--compact",
		p.getGribDir() + stamp.Filename(forecast)}

	cmd := exec.Command("grib2json/bin/grib2json", args...)
	cmd.Env = append(os.Environ())
	cmd.Stdout = lw
	cmd.Stderr = lw
	return cmd.Run()
}
