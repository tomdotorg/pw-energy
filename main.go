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
	indexData     templateData
	indexTmpl     *template.Template
	dashboardTmpl *template.Template
)

func init() {
	dbConnect()
}

func dbConnect() {
	// Capture connection properties.
	cfg := mysql.Config{
		User:                    os.Getenv("DBUSER"),
		Passwd:                  os.Getenv("DBPASS"),
		Net:                     "tcp",
		Addr:                    os.Getenv("DBADDR"),
		DBName:                  "energy",
		AllowNativePasswords:    true,
		AllowCleartextPasswords: true,
		ParseTime:               true,
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
	AsOf                time.Time
	SiteInstantPower    int
	LoadInstantPower    int
	BatteryInstantPower int
	SolarInstantPower   int
	BatteryCharge       int
	QueryTime           time.Duration
}

// energyByLocation queries for the current information for a site.
func energyByLocation(location string) (TopStats, error) {
	start := time.Now()
	var stats TopStats

	topic := "energy/" + location + "/energy"
	row := db.QueryRow("SELECT dt asof, payload->>'$.load.instant_power' ld, payload->>'$.battery.instant_power' battery, payload->>'$.site.instant_power' site, payload->>'$.solar.instant_power' solar FROM energy where topic = ? order by asOf desc limit 1;", topic)
	stats.QueryTime = time.Since(start)

	var (
		load    float64
		battery float64
		site    float64
		solar   float64
	)
	if err := row.Scan(&stats.AsOf, &load, &battery, &site, &solar); err != nil {
		if err == sql.ErrNoRows {
			return stats, fmt.Errorf("topic %s: no last row", topic)
		}
		s := fmt.Sprintf("%+v", err)
		return stats, fmt.Errorf("energyByLocation() %s", s)
	}
	stats.LoadInstantPower = int(load)
	stats.BatteryInstantPower = int(battery)
	stats.SiteInstantPower = int(site)
	stats.SolarInstantPower = int(solar)
	return stats, nil
}

// batteryByLocation queries for the current information for a site.
func batteryByLocation(location string) (pct int, dur time.Duration, err error) {
	start := time.Now()

	topic := "energy/" + location + "/battery"
	row := db.QueryRow("SELECT dt asof, payload->>'$.percentage' pct FROM energy where topic = ? order by asOf desc limit 1;", topic)
	duration := time.Since(start)
	var asOf string
	var percent float64
	if err := row.Scan(&asOf, &percent); err != nil {
		if err == sql.ErrNoRows {
			return pct, duration, fmt.Errorf("topic %s: no last row", topic)
		}
		s := fmt.Sprintf("%+v", err)
		return pct, duration, fmt.Errorf("energyByLocation() %s", s)
	}
	pct = int(percent)
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
	indexTmpl = template.Must(template.ParseFiles("index.html"))
	indexData = templateData{
		Service:  service,
		Revision: revision,
	}
	http.HandleFunc("/", helloRunHandler)

	dashboardTmpl = template.Must(template.ParseFiles("dashboard.html"))
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
	stats.BatteryCharge, duration, err = batteryByLocation("vt")
	if err != nil {
		s := fmt.Sprintf("%+v", err)
		http.Error(w, s, http.StatusInternalServerError)
	}
	stats.QueryTime += duration
	// s := fmt.Sprintf("%+v in %+v", stats, stats.queryTime)
	// w.Write([]byte(s))

	if err := dashboardTmpl.Execute(w, stats); err != nil {
		msg := http.StatusText(http.StatusInternalServerError)
		log.Printf("template.Execute: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
	}

}

// helloRunHandler responds to requests by rendering an HTML page.
func helloRunHandler(w http.ResponseWriter, r *http.Request) {
	if err := indexTmpl.Execute(w, indexData); err != nil {
		msg := http.StatusText(http.StatusInternalServerError)
		log.Printf("template.Execute: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
	}
}
