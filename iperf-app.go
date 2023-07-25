package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/robfig/cron"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

type Config struct {
	IperfServerIp string `json:"iperf_server_ip"`
	UseDB  bool   `json:"use_db"`
	DBUser string `json:"db_user"`
	DBPass string `json:"db_pass"`
	DBHost string `json:"db_host"`
	DBPort int    `json:"db_port"`
	DBName string `json:"db_name"`
}

type SpeedTest struct {
	ID         int       `json:"id"`
	Ping       float64   `json:"ping"`
	Download   float64   `json:"download"`
	Upload     float64   `json:"upload"`
	ServerID   int       `json:"server_id"`
	ServerHost string    `json:"server_host"`
	ServerName string    `json:"server_name"`
	URL        string    `json:"url"`
	Scheduled  bool      `json:"scheduled"`
	Failed     bool      `json:"failed"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type NetworkData struct {
	Upload    string `json:"upload"`
	Download  string `json:"download"`
	PingTime  string `json:"pingTime"`
	TimeStamp string `json:"timeStamp"`
}

func initializeDB(config Config) error {

	// Create the connection string
	dataSourceName := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", config.DBUser, config.DBPass, config.DBHost, config.DBPort, config.DBName)

	// Open a connection to the database
	db, err := sql.Open("mysql", dataSourceName)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// Create the table if it doesn't exist
	tableName := config.DBName
	createTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id INT AUTO_INCREMENT PRIMARY KEY,
			ping FLOAT,
			download FLOAT,
			upload FLOAT,
			timestamp DATETIME
		)`, tableName)

	_, err = db.Exec(createTableSQL)

	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	return nil
}

func parseIperfOutput(output string) (float64, float64) {
	senderBitrate := 0.0
	receiverBitrate := 0.0

	// Search for the sender and receiver bitrates in the iperf output
	senderRegex := regexp.MustCompile(`(\d+\.?\d*)\s*([GM]?)bits/sec\s*\d+\s+sender`)
	receiverRegex := regexp.MustCompile(`(\d+\.?\d*)\s*([GM]?)bits/sec\s+receiver`)

	// Find the sender bitrate
	senderMatch := senderRegex.FindStringSubmatch(output)
	if senderMatch != nil {
		senderBitrate = parseFloat(senderMatch[1])
	}

	// Find the receiver bitrate
	receiverMatch := receiverRegex.FindStringSubmatch(output)
	if receiverMatch != nil {
		receiverBitrate = parseFloat(receiverMatch[1])
	}

	return senderBitrate, receiverBitrate
}

func saveJSON(upload float64, download float64, pingTime float64) error {
	uploadSpeed := fmt.Sprintf("%.3f", upload)
	downloadSpeed := fmt.Sprintf("%.3f", download)
	pingContent := fmt.Sprintf("%.3f", pingTime)
	currentTime := time.Now().Format(time.RFC3339)

	data := NetworkData{
		Upload:    uploadSpeed,
		Download:  downloadSpeed,
		PingTime:  pingContent,
		TimeStamp: currentTime,
	}

	jsonData, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		log.Println("Error marshaling JSON:", err)
		return err
	}

	err = ioutil.WriteFile("netdata.json", jsonData, 0644)
	if err != nil {
		log.Println("Error writing JSON to file:", err)
		return err
	}

	return nil
}

func parseFloat(s string) float64 {
	value, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Println("Error parsing float:", err)
	}
	return value
}

func runIperfCommand(serverIP string) (float64, float64, error) {
	cmd := exec.Command("iperf3", "-c", serverIP)

	// Execute the command and wait for it to finish
	output, err := cmd.Output()
	if err != nil {
		log.Println("Error running iperf3 command:", err)
		return 0, 0, err
	}

	sender, receiver := parseIperfOutput(string(output))
	return sender, receiver, nil
}

func pingIP(serverIP string) (float64, error) {
	cmd := exec.Command("ping", "-c", "5", serverIP)

	output, err := cmd.Output()
	if err != nil {
		log.Println("Error running ping command:", err)
		return 0, err
	}

	outputStr := string(output)
	pingRegex := regexp.MustCompile(`time=(\d+\.?\d*)`)
	pingMatches := pingRegex.FindAllStringSubmatch(outputStr, -1)

	totalTime := 0.0
	for _, match := range pingMatches {
		pingTime, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			log.Println("Error parsing ping time:", err)
			continue
		}
		totalTime += pingTime
	}

	avgPingTime := totalTime / float64(len(pingMatches))
	return avgPingTime, nil
}

func handleSpeedTestLatest(w http.ResponseWriter, r *http.Request) {
	// Redirect if invalid
	if r.URL.Path != "/api/speedtest/latest" {
		http.Redirect(w, r, "/api/speedtest/latest", http.StatusMovedPermanently)
		return
	}

	log.Printf("%s - - [%s] \"%s %s %s\" %d %d \"%s\" \"%s\"\n",
		r.RemoteAddr,
		time.Now().Format("02/Jan/2006:15:04:05 -0700"),
		r.Method,
		r.RequestURI,
		r.Proto,
		http.StatusOK,
		261,
		r.Referer(),
		r.UserAgent(),
	)

	// Read JSON file
	data, err := ioutil.ReadFile("netdata.json")
	if err != nil {
		log.Println("Error reading file:", err)
		http.Error(w, "Failed to read file", http.StatusInternalServerError)
		return
	}

	var netData NetworkData
	err = json.Unmarshal(data, &netData)
	if err != nil {
		log.Println("Error decoding JSON:", err)
		http.Error(w, "Failed to decode JSON", http.StatusInternalServerError)
		return
	}

	pingTime, err := strconv.ParseFloat(netData.PingTime, 64)
	if err != nil {
		log.Println("Error parsing PingTime:", err)
		http.Error(w, "Failed to parse PingTime", http.StatusInternalServerError)
		return
	}

	download, err := strconv.ParseFloat(netData.Download, 64)
	if err != nil {
		log.Println("Error parsing Download:", err)
		http.Error(w, "Failed to parse Download", http.StatusInternalServerError)
		return
	}

	upload, err := strconv.ParseFloat(netData.Upload, 64)
	if err != nil {
		log.Println("Error parsing Upload:", err)
		http.Error(w, "Failed to parse Upload", http.StatusInternalServerError)
		return
	}

	var timeStamp time.Time

	layouts := []string{"2006-01-02T15:04:05-07:00", "2006-01-02T15:04:05Z"}

	for _, layout := range layouts {
		timeStamp, err = time.Parse(layout, netData.TimeStamp)
		if err == nil {
			break
		}
	}

	if err != nil {
		log.Println("Error parsing TimeStamp:", err)
		http.Error(w, "Failed to parse TimeStamp", http.StatusInternalServerError)
		return
	}

	speedTest := SpeedTest{
		ID:         1,
		Ping:       roundDecimals(pingTime, 3),
		Download:   roundDecimals(download, 4),
		Upload:     roundDecimals(upload, 4),
		ServerID:   1,
		ServerHost: "iperf.lan",
		ServerName: "iperf",
		URL:        "http://iperf.lan/api/speedtest.latest",
		Scheduled:  true,
		Failed:     true,
		CreatedAt:  timeStamp,
		UpdatedAt:  timeStamp,
	}

	// Create the API response object
	apiResponse := struct {
		Message string    `json:"message"`
		Data    SpeedTest `json:"data"`
	}{
		Message: "ok",
		Data:    speedTest,
	}

	// Marshal the JSON response
	response, err := json.Marshal(apiResponse)
	if err != nil {
		log.Println("Error marshaling JSON:", err)
		http.Error(w, "Failed to marshal JSON response", http.StatusInternalServerError)
		return
	}

	// Set the Content-Type header to application/json
	w.Header().Set("Content-Type", "application/json")
	w.Write(response)
}

// roundDecimals rounds a float64 value to the specified number of decimal places
func roundDecimals(value float64, decimals int) float64 {
	shift := math.Pow(10, float64(decimals))
	return math.Round(value*shift) / shift
}

func handleRedirect(w http.ResponseWriter, r *http.Request) {
	// Redirect to /api/speedtest/latest
	http.Redirect(w, r, "/api/speedtest/latest", http.StatusMovedPermanently)
}

func main() {
	// Read the configuration file
	configFile, err := ioutil.ReadFile("config.json")
	if err != nil {
		log.Fatal("Failed to read config file:", err)
	}

	// Parse the configuration values
	var config Config
	err = json.Unmarshal(configFile, &config)
	if err != nil {
		log.Fatal("Failed to parse config file:", err)
	}

	// Override config values with environment variables
	iperfServerIPEnv := os.Getenv("IPERF_SERVER_IP")
	if iperfServerIPEnv != "" {
		config.IperfServerIp = iperfServerIPEnv
	}

	useDBEnv := os.Getenv("USE_DB")
	if useDBEnv != "" {
		useDB, err := strconv.ParseBool(useDBEnv)
		if err == nil {
			config.UseDB = useDB
		}
	}

	dbUserEnv := os.Getenv("MYSQL_USER")
	if dbUserEnv != "" {
		config.DBUser = dbUserEnv
	}

	dbPassEnv := os.Getenv("MYSQL_PASSWORD")
	if dbPassEnv != "" {
		config.DBPass = dbPassEnv
	}

	dbHostEnv := os.Getenv("DB_HOST")
	if dbHostEnv != "" {
		config.DBHost = dbHostEnv
	}

	dbPortEnv := os.Getenv("DB_PORT")
	if dbPortEnv != "" {
		dbPort, err := strconv.Atoi(dbPortEnv)
		if err == nil {
			config.DBPort = dbPort
		}
	}

	dbNameEnv := os.Getenv("MYSQL_DATABASE")
	if dbNameEnv != "" {
		config.DBName = dbNameEnv
	}

	// Update the config.json file with the modified values
	configJSON, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		log.Fatal("Failed to marshal config JSON:", err)
	}

	err = ioutil.WriteFile("config.json", configJSON, 0644)
	if err != nil {
		log.Fatal("Failed to write config file:", err)
	}

	if config.UseDB == true {

		// Initialize the database
		err = initializeDB(config)
		if err != nil {
			log.Fatal("Failed to initialize the database:", err)
		}
	}

	// Create a new cron scheduler
	c := cron.New()

	// Run every hour on the hour
	c.AddFunc("0 0 * * * *", func() {
		sender, receiver, err := runIperfCommand(config.IperfServerIp)
		if err != nil {
			log.Println("Failed to run iperf3 command:", err)
		}

		pingTime, err := pingIP(config.IperfServerIp)
		if err != nil {
			log.Println("Failed to ping IP:", err)
		}

		err = saveJSON(sender, receiver, pingTime)
		if err != nil {
			log.Println("Failed to save ping time:", err)
		}

		if config.UseDB == true {
			// if true store data in mariadb
			err = storeDataInDB(config)
			if err != nil {
				log.Println("Error storing data in database:", err)
				return
			}

			log.Println("Data stored successfully:", sender, receiver, pingTime)
		}
	})

	// Start the cron scheduler
	c.Start()

	http.HandleFunc("/api/speedtest/latest", handleSpeedTestLatest)
	http.HandleFunc("/", handleRedirect)
	fmt.Println("Server listening on http://localhost:8000/api/speedtest/latest")
	log.Fatal(http.ListenAndServe(":8000", nil))

	// Keep the program running
	select {}
}

func storeDataInDB(config Config) error {
	// Create the connection string
	dataSourceName := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", config.DBUser, config.DBPass, config.DBHost, config.DBPort, config.DBName)

	// Read JSON file
	jsonData, err := ioutil.ReadFile("netdata.json")
	if err != nil {
		return fmt.Errorf("failed to read JSON file: %w", err)
	}

	// Parse JSON data into NetworkData struct
	var networkData NetworkData
	err = json.Unmarshal(jsonData, &networkData)
	if err != nil {
		return fmt.Errorf("failed to parse JSON data: %w", err)
	}

	// Open a connection to the database
	db, err := sql.Open("mysql", dataSourceName)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// Prepare the SQL statement
	tableName := config.DBName
	stmt, err := db.Prepare("INSERT INTO " + tableName + "(ping, download, upload, timestamp) VALUES (?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare SQL statement: %w", err)
	}
	defer stmt.Close()

	// Format the timestamp in MySQL datetime format
	timestamp, err := time.Parse(time.RFC3339, networkData.TimeStamp)
	if err != nil {
		return fmt.Errorf("failed to parse timestamp: %w", err)
	}
	formattedTimestamp := timestamp.Format("2006-01-02 15:04:05")

	// Execute the SQL statement
	_, err = stmt.Exec(networkData.PingTime, networkData.Download, networkData.Upload, formattedTimestamp)
	if err != nil {
		return fmt.Errorf("failed to execute SQL statement: %w", err)
	}

	return nil
}
