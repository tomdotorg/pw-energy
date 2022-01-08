package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
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
	dbName := os.Getenv("DB_NAME")
	user := os.Getenv("DB_USER")
	passwd := os.Getenv("DB_PASS")
	instanceConnectionName := os.Getenv("INSTANCE_CONNECTION_NAME")
	socketDir, isSet := os.LookupEnv("DB_SOCKET_DIR")
	if !isSet {
		socketDir = "/cloudsql"
	}

	dbURI := fmt.Sprintf("%s:%s@unix(%s/%s)/%s?parseTime=true",
		user, passwd, socketDir, instanceConnectionName, dbName)
	log.Println(dbURI)
	// dbPool is the pool of database connections.
	var err error
	db, err = sql.Open("mysql", dbURI)
	if err != nil {
		log.Fatalf("sql.Open(): %v", err)
	}
	// db, err = sql.Open("mysql", cfg.FormatDSN())
	// if err != nil {
	// 	log.Fatal(err)
	// }
	//
	pingErr := db.Ping()
	if pingErr != nil {
		log.Fatal(pingErr)
	}
	log.Println("Connected!")
}

// func mongoConnect() *mongo.Client {
// 	connString := "mongodb+srv://" + os.Getenv("DBUSER") + ":" + os.Getenv("DBPASS") + "@" + os.Getenv("DBHOST") + "/" + os.Getenv("DBNAME")
// 	log.Printf("connecting with: [%s]", connString)
// 	clientOptions := options.Client().ApplyURI(connString)
// 	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
// 	defer cancel()
// 	client, err := mongo.Connect(ctx, clientOptions)
// 	if err != nil {
// 		log.Fatal(err)
// 	} // Capture connection properties.
// 	// Get a database handle.
// 	log.Println("Database connected!")
// 	return client
// }

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
func energyByLocation(locations ...string) ([]TopStats, error) {
	start := time.Now()
	var allStats []TopStats = make([]TopStats, 0)

	for _, location := range locations {
		var stats TopStats
		topic := "energy/" + location + "/energy"
		row := db.QueryRow("SELECT dt asof, payload->>'$.load.instant_power' ld, payload->>'$.battery.instant_power' battery, payload->>'$.site.instant_power' site, payload->>'$.solar.instant_power' solar FROM energy where topic = ? order by asOf desc limit 1;", topic)

		var (
			load    float64
			battery float64
			site    float64
			solar   float64
		)
		if err := row.Scan(&stats.AsOf, &load, &battery, &site, &solar); err != nil {
			if err == sql.ErrNoRows {
				return allStats, fmt.Errorf("topic %s: no last row", topic)
			}
			s := fmt.Sprintf("%+v", err)
			return allStats, fmt.Errorf("energyByLocation() %s", s)
		}
		stats.QueryTime = time.Since(start)
		stats.LoadInstantPower = int(load)
		stats.BatteryInstantPower = int(battery)
		stats.SiteInstantPower = int(site)
		stats.SolarInstantPower = int(solar)
		allStats = append(allStats, stats)
	}
	return allStats, nil
}

// batteryByLocation queries for the current information for a site.
func batteryByLocation(location string) (pct int, dur time.Duration, err error) {
	start := time.Now()

	topic := "energy/" + location + "/battery"
	row := db.QueryRow("SELECT dt asof, payload->>'$.percentage' pct FROM energy where topic = ? order by asOf desc limit 1;", topic)
	var asOf string
	var percent float64
	if err := row.Scan(&asOf, &percent); err != nil {
		if err == sql.ErrNoRows {
			return pct, 0, fmt.Errorf("topic %s: no last row", topic)
		}
		s := fmt.Sprintf("%+v", err)
		return pct, 0, fmt.Errorf("energyByLocation() %s", s)
	}
	duration := time.Since(start)
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
	http.HandleFunc("/energy", energyHandler)

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
	stats := make([]TopStats, 0)
	stats, err := energyByLocation("ma", "vt")
	if err != nil {
		s := fmt.Sprintf("%+v", err)
		http.Error(w, s, http.StatusInternalServerError)
	}

	var duration time.Duration
	stats[0].BatteryCharge, duration, err = batteryByLocation("ma")
	if err != nil {
		s := fmt.Sprintf("%+v", err)
		http.Error(w, s, http.StatusInternalServerError)
	}
	stats[1].BatteryCharge, duration, err = batteryByLocation("vt")
	if err != nil {
		s := fmt.Sprintf("%+v", err)
		http.Error(w, s, http.StatusInternalServerError)
	}
	stats[0].QueryTime += duration
	stats[1].QueryTime += duration
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
