function updateWeather() {
    fetch('/weather')
        .then(response => response.json())
        .then(data => {
            data.forEach(city => {
                document.getElementById(`${city.City.toLowerCase()}-main`).textContent = city.DominantCondition;
                document.getElementById(`${city.City.toLowerCase()}-temp`).textContent = city.Temperature.toFixed(1);
                document.getElementById(`${city.City.toLowerCase()}-feels-like`).textContent = city.AvgTemperature.toFixed(1);
                document.getElementById(`${city.City.toLowerCase()}-dt`).textContent = new Date(city.LastUpdated).toLocaleString();
            });
        })
        .catch(error => console.error('Error fetching weather data:', error));
}

// Update weather data every 15 seconds
setInterval(updateWeather, 30000);

// Initial update
updateWeather();