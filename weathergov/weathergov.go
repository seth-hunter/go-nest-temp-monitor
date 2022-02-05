package weathergov

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"

	"github.com/blakehartshorn/go-nest-temp-monitor/configuration"
)

// Response - root of JSON object returned by weather.gov
type Response struct {
	Properties struct {
		Timestamp   string  `json:"timestamp"`
		Temperature Weather `json:"temperature"`
		Humidity    Weather `json:"relativeHumidity"`
		Pressure    Weather `json:"barometricPressure"`
	} `json:"properties"`
}

// Weather - parse json data for value and unit
type Weather struct {
	Value          float64 `json:"value"`
	UnitCode       string  `json:"unitCode"`
	QualityControl string  `json:"qualityControl"`
}

// WriteWeather - write weather metrics to InfluxDB
func WriteWeather(
	config configuration.WeatherGovConfig,
	influx api.WriteAPI) {
	url := fmt.Sprintf(
		"https://api.weather.gov/stations/%s/observations/latest",
		config.Station,
	)
	for {
		httpClient := &http.Client{Timeout: time.Second * 10}
		res, err := httpClient.Get(url)
		if err != nil {
			log.Printf("ERROR while featching weather.gov data: %s\n", err)
			time.Sleep(time.Minute * time.Duration(config.Interval))
			continue
		}
		defer res.Body.Close()

		body, err := ioutil.ReadAll(res.Body)

		var weather Response
		jsonErr := json.Unmarshal(body, &weather)
		if jsonErr != nil {
			log.Println("ERROR: Could not unmarshal json!")
			log.Print(jsonErr)
		} else {

			var fields = make(map[string]interface{})

			// weather.gov sometimes reports a value of 0 when it doesn't have data.
			// Given that 0 humidity never happens, 0 pressure means we all die,
			// and a floating point value being exactly 0 for temperature is rare,
			// it's better to pass null values instead.
			timestamp, _ := time.Parse(time.RFC3339, weather.Properties.Timestamp)
			if weather.Properties.Temperature.Value != 0 {
				fields["temperature"] = weather.Properties.Temperature.Value
			}
			if weather.Properties.Humidity.Value > 0 {
				fields["humidity"] = weather.Properties.Humidity.Value
			}
			if weather.Properties.Pressure.Value > 0 {
				// Convert Pa to hPa for consistency with other apps
				fields["pressure"] = weather.Properties.Pressure.Value * 0.01
			}

			p := influxdb2.NewPoint("weathergov",
				map[string]string{
					"station": config.Station,
				},
				fields, timestamp)
			influx.WritePoint(p)

			log.Printf("Wrote weather metrics from weather.gov. Sleeping for %d minute(s).\n", config.Interval)
		}
		time.Sleep(time.Minute * time.Duration(config.Interval))
	}
}
