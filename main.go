package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"text/template"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type WeatherData struct {
	City              string    `bson:"city"`
	Date              time.Time `bson:"date"`
	Temperature       float64   `bson:"temperature"`
	AvgTemperature    float64   `bson:"avg_temperature"`
	DominantCondition string    `bson:"dominant_condition"`
	LastUpdated       time.Time `bson:"last_updated"`
	MaxTemperature    float64   `bson:"max_temperature"`
	MinTemperature    float64   `bson:"min_temperature"`
	Alert             bool      `json:"alert"`
}

type DailySummary struct {
	City              string    `bson:"city"`
	Date              time.Time `bson:"date"`
	AvgTemperature    float64   `bson:"avg_temperature"`
	MaxTemperature    float64   `bson:"max_temperature"`
	MinTemperature    float64   `bson:"min_temperature"`
	DominantCondition string    `bson:"dominant_condition"`
}

type WeatherResponse struct {
	Weather []struct {
		Main string `json:"main"`
	} `json:"weather"`
	Main struct {
		Temp      float64 `json:"temp"`
		FeelsLike float64 `json:"feels_like"`
	} `json:"main"`
	Dt int64 `json:"dt"`
}

const temperatureThreshold = 28.0

func calculateAndStoreDailySummary(client *mongo.Client, city string) error {
	collection := client.Database("weather_monitoring").Collection("weather_data")
	summaryCollection := client.Database("weather_monitoring").Collection("daily_summaries")

	// Get the start and end of the current day in UTC
	now := time.Now().UTC()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour)

	// Query for today's weather data
	filter := bson.M{
		"city": city,
		"date": bson.M{
			"$gte": startOfDay,
			"$lt":  endOfDay,
		},
	}

	cursor, err := collection.Find(context.TODO(), filter)
	if err != nil {
		return err
	}
	defer cursor.Close(context.TODO())

	var weatherData []WeatherData
	if err = cursor.All(context.TODO(), &weatherData); err != nil {
		return err
	}

	if len(weatherData) == 0 {
		return nil // No data for today, skip summary
	}

	// Calculate aggregates
	var sumTemp, maxTemp, minTemp float64
	conditionCounts := make(map[string]int)

	for i, data := range weatherData {
		sumTemp += data.Temperature
		if i == 0 || data.Temperature > maxTemp {
			maxTemp = data.Temperature
		}
		if i == 0 || data.Temperature < minTemp {
			minTemp = data.Temperature
		}
		conditionCounts[data.DominantCondition]++
	}

	avgTemp := sumTemp / float64(len(weatherData))

	// Determine dominant condition
	var dominantCondition string
	maxCount := 0
	for condition, count := range conditionCounts {
		if count > maxCount {
			maxCount = count
			dominantCondition = condition
		}
	}

	// Create and store the daily summary
	summary := DailySummary{
		City:              city,
		Date:              startOfDay,
		AvgTemperature:    avgTemp,
		MaxTemperature:    maxTemp,
		MinTemperature:    minTemp,
		DominantCondition: dominantCondition,
	}

	_, err = summaryCollection.UpdateOne(
		context.TODO(),
		bson.M{"city": city, "date": startOfDay},
		bson.M{"$set": summary},
		options.Update().SetUpsert(true),
	)

	return err
}

func getWeather(apiURL string) (WeatherResponse, error) {
	resp, err := http.Get(apiURL)
	if err != nil {
		return WeatherResponse{}, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return WeatherResponse{}, err
	}
	// fmt.Println("Body: ", string(body))

	var weatherResponse WeatherResponse
	err = json.Unmarshal(body, &weatherResponse)
	if err != nil {
		return WeatherResponse{}, err
	}

	return weatherResponse, nil
}

func storeWeatherData(client *mongo.Client, data WeatherData) error {
	collection := client.Database("weather_monitoring").Collection("weather_data")
	filter := bson.M{"city": data.City}
	update := bson.M{
		"$set": bson.M{
			"date":               data.Date,
			"temperature":        data.Temperature,
			"avg_temperature":    data.AvgTemperature,
			"dominant_condition": data.DominantCondition,
			"last_updated":       data.LastUpdated,
			"max_temperature":    data.MaxTemperature,
			"min_temperature":    data.MinTemperature,
		},
	}

	opts := options.Update().SetUpsert(true)

	_, err := collection.UpdateOne(context.TODO(), filter, update, opts)
	return err
}

func getLastWeatherData(client *mongo.Client, city string) (WeatherData, error) {
	var lastData WeatherData
	collection := client.Database("weather_monitoring").Collection("weather_data")
	filter := bson.M{"city": city}
	err := collection.FindOne(context.TODO(), filter).Decode(&lastData)
	if err == mongo.ErrNoDocuments {
		// If no document found, return a WeatherData with default values
		return WeatherData{City: city, MinTemperature: math.Inf(1), MaxTemperature: math.Inf(-1)}, nil
	}
	return lastData, err
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, nil)
}

func main() {
	cities := []string{"Mumbai", "Delhi", "Bangalore", "Hyderabad", "Chennai", "Kolkata"}
	apiKey := "1616f536a0ea3a64d71aa16741e957a2"
	clientOptions := options.Client().ApplyURI("mongodb+srv://weather:Vaib9837@weathermonitoringcluste.3wudy.mongodb.net/?retryWrites=true&w=majority&appName=WeatherMonitoringCluster")
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		log.Fatal(err)
	}
	fs := http.FileServer(http.Dir("."))
	http.Handle("/style.css", fs)
	http.Handle("/script.js", fs)
	// http.Handle("/script.js", fs)
	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/weather", func(w http.ResponseWriter, r *http.Request) {
		handleWeather(w, r, client)
	})
	go weatherMonitoring(client, cities, apiKey)

	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleWeather(w http.ResponseWriter, _ *http.Request, client *mongo.Client) {
	collection := client.Database("weather_monitoring").Collection("weather_data")
	cursor, err := collection.Find(context.TODO(), bson.M{})
	if err != nil {
		http.Error(w, "Failed to fetch weather data", http.StatusInternalServerError)
		return
	}
	defer cursor.Close(context.TODO())

	var weatherData []WeatherData
	if err = cursor.All(context.TODO(), &weatherData); err != nil {
		http.Error(w, "Failed to decode weather data", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(weatherData)
}

func weatherMonitoring(client *mongo.Client, cities []string, apiKey string) {
	for {
		for _, city := range cities {
			apiURL := fmt.Sprintf("https://api.openweathermap.org/data/2.5/weather?q=%s&APPID=%s&units=metric", city, apiKey)
			weather, err := getWeather(apiURL)
			if err != nil {
				fmt.Printf("Error fetching weather for %s: %v\n", city, err)
				continue
			}

			lastData, err := getLastWeatherData(client, city)
			if err != nil {
				fmt.Printf("Error fetching last weather data for %s: %v\n", city, err)
				continue
			}

			// Calculate min and max temperature
			minTemp := math.Min(lastData.MinTemperature, weather.Main.Temp) // Example calculation
			maxTemp := math.Max(lastData.MaxTemperature, weather.Main.Temp) // Example calculation
			avgTemp := (minTemp + maxTemp) / 2

			weatherData := WeatherData{
				City:              city,
				Date:              time.Unix(time.Now().UTC().Unix(), 0),
				Temperature:       weather.Main.Temp,
				DominantCondition: weather.Weather[0].Main,
				LastUpdated:       time.Now().UTC(),
				MaxTemperature:    maxTemp,
				MinTemperature:    minTemp,
				AvgTemperature:    avgTemp,
				Alert:             weather.Main.Temp > temperatureThreshold,
			}
			err = storeWeatherData(client, weatherData)
			if err != nil {
				fmt.Printf("Error storing weather data for %s: %v\n", city, err)
			} else {
				fmt.Printf("Stored weather data for %s\n", city)
			}
			err = calculateAndStoreDailySummary(client, city)
			if err != nil {
				fmt.Printf("Error calculating daily summary for %s: %v\n", city, err)
			} else {
				fmt.Printf("Calculated and stored daily summary for %s\n", city)
			}
		}
		time.Sleep(30 * time.Second)
	}
}
