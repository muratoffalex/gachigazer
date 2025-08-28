package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"time"
)

type WeatherData struct {
	CurrentCondition []struct {
		TempC       string `json:"temp_C"`
		WeatherDesc []struct {
			Value string `json:"value"`
		} `json:"weatherDesc"`
		WindspeedKmph string `json:"windspeedKmph"`
	} `json:"current_condition"`
	Weather []struct {
		Date   string `json:"date"`
		Hourly []struct {
			Time        string `json:"time"`
			TempC       string `json:"tempC"`
			WeatherDesc []struct {
				Value string `json:"value"`
			} `json:"weatherDesc"`
			WindspeedKmph string `json:"windspeedKmph"`
		} `json:"hourly"`
	} `json:"weather"`
}

func (t Tools) Weather(location string, days int) (string, error) {
	if days < 1 || days > 7 {
		days = 1
	}

	url := fmt.Sprintf("https://wttr.in/%s?format=j1", location)
	resp, err := t.httpClient.Get(url)
	if err != nil {
		return "Weather service unavailable", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "Failed to read weather data", err
	}

	var data WeatherData
	if err := json.Unmarshal(body, &data); err != nil {
		return "Failed to parse weather data", err
	}

	today := time.Now()
	result := fmt.Sprintf("Weather forecast for %s:\n", location)

	// Add current weather from current_condition
	if len(data.CurrentCondition) > 0 {
		current := data.CurrentCondition[0]
		result += fmt.Sprintf("\nCurrent: %s°C, %s, wind %s km/h\n",
			current.TempC,
			current.WeatherDesc[0].Value,
			current.WindspeedKmph)
	}

	for i := 0; i < days && i < len(data.Weather); i++ {
		day := data.Weather[i]
		date, _ := time.Parse("2006-01-02", day.Date)

		dayFormat := "Monday, January 2"
		if date.YearDay() == today.YearDay() && date.Year() == today.Year() {
			dayFormat = "Today, January 2"
		}
		result += fmt.Sprintf("\n%s:\n", date.Format(dayFormat))

		timeSlots := map[string][]string{
			"Night":     {"0", "300", "600"},
			"Morning":   {"900", "1200"},
			"Afternoon": {"1500"},
			"Evening":   {"1800", "2100"},
		}

		for period, times := range timeSlots {
			var found bool
			for _, hour := range day.Hourly {
				if slices.Contains(times, hour.Time) {
					result += fmt.Sprintf("- %s: %s°C, %s, wind %s km/h\n",
						period,
						hour.TempC,
						hour.WeatherDesc[0].Value,
						hour.WindspeedKmph)
					found = true
				}
				if found {
					break
				}
			}
		}
	}

	return result, nil
}
