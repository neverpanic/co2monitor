package main

import (
	"crypto/sha1"
	"fmt"
	"log"
	"net/http"

	"github.com/larsp/co2monitor/meter"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	listenAddr = kingpin.Arg("listen-address", "The address to listen on for HTTP requests.").
			Default(":8080").String()
)

type Gauge struct {
	temperature prometheus.Gauge
	co2         prometheus.Gauge
	source      *meter.Meter
}

func main() {
	kingpin.Parse()
	deviceInfo := meter.Enumerate()

	http.Handle("/metrics", promhttp.Handler())
	go measure(deviceInfo)
	log.Printf("Serving metrics at '%v/metrics'", *listenAddr)
	log.Fatal(http.ListenAndServe(*listenAddr, nil))
}

func measure(deviceInfo []meter.HidDeviceInfo) {
	var gauges []*Gauge

	for _, device := range deviceInfo {
		gauge := new(Gauge)

		gauge.source = new(meter.Meter)
		err := gauge.source.Open(device)
		if err != nil {
			log.Fatalf("Could not open device at path '%v'", device.Path)
			return
		}

		// Register this sensor with prometheus
		// We need a unique identifier for each sensor. Unfortunately it seems
		// a lot of the CO2 sensors come with the same serial numbers, so the
		// serial cannot be used. We can use the path instead; this comes with
		// the downside that replugging the same sensor into a different slot
		// will give it a different ID. On macOS, these identifiers are very
		// long, contain spaces and special characters, so we cannot use it
		// as-is either. Hash it to get an identifier that prometheus will
		// accept.
		identifier := sha1.Sum([]byte(device.Path))
		gauge.temperature = prometheus.NewGauge(prometheus.GaugeOpts{
			Name: fmt.Sprintf("meter_%x_temperature_celsius", identifier),
			Help: fmt.Sprintf("Current temperature in Celsius for device %x", identifier),
		})
		prometheus.MustRegister(gauge.temperature)

		gauge.co2 = prometheus.NewGauge(prometheus.GaugeOpts{
			Name: fmt.Sprintf("meter_%x_co2_ppm", identifier),
			Help: fmt.Sprintf("Current CO2 level (ppm) for device %x", identifier),
		})
		prometheus.MustRegister(gauge.co2)

		gauges = append(gauges, gauge)
	}

	for {
		for _, gauge := range gauges {
			result, err := gauge.source.Read()
			if err != nil {
				log.Fatalf("Failed to read gauge: '%v'", err)
			}

			gauge.temperature.Set(result.Temperature)
			gauge.co2.Set(float64(result.Co2))
		}
	}
}
