package utils

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"
)

type Stamp struct {
	Date string
	Hour string
}

func (s *Stamp) FromNow(h int) int {
	t, err := time.Parse("2006010215", s.Date+s.Hour)
	if err != nil {
		log.Fatalln("Error", err)
		return 0
	}

	t = t.Add(time.Hour * time.Duration(h))

	forecastHour := int(math.Round(t.Sub(time.Now()).Hours()))

	return forecastHour
}

func StampFromTime(t time.Time) Stamp {
	p := t.Add(time.Hour * time.Duration(-1*(t.UTC().Hour()%6)))

	return Stamp{Date: p.UTC().Format("20060102"), Hour: p.UTC().Format("15")}
}

func StampFromFile(f string) Stamp {
	d := strings.Split(f, ".")[0]

	return Stamp{Date: d[0 : len(d)-2], Hour: d[len(d)-2:]}
}

func (s *Stamp) Filename(h int) string {
	return s.Date + s.Hour + ".f" + fmt.Sprintf("%03d", h)
}

func (s *Stamp) Key(h int) string {
	t, err := time.Parse("2006010215", s.Date+s.Hour)
	if err != nil {
		log.Fatalln("Error", err)
		return ""
	}

	t = t.Add(time.Hour * time.Duration(h))

	return t.Format("2006010215")
}
