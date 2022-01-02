package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-sql-driver/mysql"
)

var db *sql.DB

// templateData provides template parameters.
type templateData struct {
	Service  string
	Revision string
	Stats    TopStats
}

// Variables used to generate the HTML page.
var (
	data templateData
	tmpl *template.Template
)

func init() {
	dbConnect()
}

func dbConnect() {
	// Capture connection properties.
	cfg := mysql.Config{
		User:                 os.Getenv("DBUSER"),
		Passwd:               os.Getenv("DBPASS"),
		Net:                  "tcp",
		Addr:                 os.Getenv("DBADDR"),
		DBName:               "energy",
		AllowNativePasswords: true,
		ParseTime:            true,
	}
	// Get a database handle.
	var err error
	log.Printf("%+v", cfg)
	db, err = sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		log.Fatal(err)
	}

	pingErr := db.Ping()
	if pingErr != nil {
		log.Fatal(pingErr)
	}
	log.Println("Database connected!")
}

type TopStats struct {
	asOf                time.Time
	siteInstantPower    float64
	loadInstantPower    float64
	batteryInstantPower float64
	solarInstantPower   float64
	batteryCharge       float64
	queryTime           time.Duration
}

// energyByLocation queries for the current information for a site.
func energyByLocation(location string) (TopStats, error) {
	// An album to hold data from the returned row.
	start := time.Now()
	var stats TopStats

	topic := "energy/" + location + "/energy"
	row := db.QueryRow("SELECT dt asof, payload->>'$.load.instant_power' ld, payload->>'$.battery.instant_power' battery, payload->>'$.site.instant_power' site, payload->>'$.solar.instant_power' solar FROM energy where topic = ? order by asOf desc limit 1;", topic)
	stats.queryTime = time.Since(start)

	if err := row.Scan(&stats.asOf, &stats.loadInstantPower, &stats.batteryInstantPower, &stats.siteInstantPower, &stats.solarInstantPower); err != nil {
		if err == sql.ErrNoRows {
			return stats, fmt.Errorf("topic %s: no last row", topic)
		}
		s := fmt.Sprintf("%+v", err)
		return stats, fmt.Errorf("energyByLocation() %s", s)
	}
	return stats, nil
}

// batteryByLocation queries for the current information for a site.
func batteryByLocation(location string) (pct float64, dur time.Duration, err error) {
	// An album to hold data from the returned row.
	start := time.Now()

	topic := "energy/" + location + "/battery"
	row := db.QueryRow("SELECT dt asof, payload->>'$.percentage' pct FROM energy where topic = ? order by asOf desc limit 1;", topic)
	duration := time.Since(start)
	var asOf string
	if err := row.Scan(&asOf, &pct); err != nil {
		if err == sql.ErrNoRows {
			return pct, duration, fmt.Errorf("topic %s: no last row", topic)
		}
		s := fmt.Sprintf("%+v", err)
		return pct, duration, fmt.Errorf("energyByLocation() %s", s)
	}
	return pct, duration, nil
}

func main() {
	// Initialize template parameters.
	service := os.Getenv("K_SERVICE")
	if service == "" {
		service = "???"
	}

	revision := os.Getenv("K_REVISION")
	if revision == "" {
		revision = "???"
	}

	// Prepare template for execution.
	tmpl = template.Must(template.ParseFiles("index.html"))
	data = templateData{
		Service:  service,
		Revision: revision,
	}

	// Define HTTP server.
	http.HandleFunc("/", helloRunHandler)
	http.HandleFunc("/energy-vt", energyHandler)

	fs := http.FileServer(http.Dir("./assets"))
	http.Handle("/assets/", http.StripPrefix("/assets/", fs))

	// PORT environment variable is provided by Cloud Run.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Print("Hello from Cloud Run! The container started successfully and is listening for HTTP requests on $PORT")
	log.Printf("Listening on port %s", port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatal(err)
	}
}

func energyHandler(w http.ResponseWriter, r *http.Request) {
	stats, err := energyByLocation("vt")
	if err != nil {
		s := fmt.Sprintf("%+v", err)
		http.Error(w, s, http.StatusInternalServerError)
	}

	var duration time.Duration
	stats.batteryCharge, duration, err = batteryByLocation("vt")
	if err != nil {
		s := fmt.Sprintf("%+v", err)
		http.Error(w, s, http.StatusInternalServerError)
	}
	stats.queryTime += duration
	s := fmt.Sprintf("%+v in %+v", stats, stats.queryTime)
	w.Write([]byte(s))

}

// helloRunHandler responds to requests by rendering an HTML page.
func helloRunHandler(w http.ResponseWriter, r *http.Request) {
	if err := tmpl.Execute(w, data); err != nil {
		msg := http.StatusText(http.StatusInternalServerError)
		log.Printf("template.Execute: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
	}
}
