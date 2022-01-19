package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/rs/zerolog/pkgerrors"
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
	if err := os.Setenv("TZ", "America/New_York"); err != nil {
		log.Fatal().Str("TZ", os.Getenv("TZ")).Msg("missing or invalid TZ")
	}
}

func dbConnect() {
	dbName := os.Getenv("DB_NAME")
	user := os.Getenv("DB_USER")
	passwd := os.Getenv("DB_PASS")
	instanceConnectionName := os.Getenv("INSTANCE_CONNECTION_NAME")
	socketDir, isSet := os.LookupEnv("DB_SOCKET_DIR")
	// log.Debug("env", "DB_NAME", dbName).Msg("")
	if !isSet {
		socketDir = "/cloudsql"
	}

	dbURI := fmt.Sprintf("%s:%s@unix(%s/%s)/%s?parseTime=true",
		user, passwd, socketDir, instanceConnectionName, dbName)
	// log.Print(dbURI)
	// dbPool is the pool of database connections.
	var err error
	db, err = sql.Open("mysql", dbURI)
	if err != nil {
		log.Fatal().Err(err).Msg("sql.Open()")
	}
	// db, err = sql.Open("mysql", cfg.FormatDSN())
	// if err != nil {
	// 	log.Fatal(err)
	// }
	//
	pingErr := db.Ping()
	if pingErr != nil {
		log.Fatal().Err(pingErr).Msg("pinging db")
	}
	log.Print("Connected!")
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
	Location            string
	AsOf                time.Time
	SiteInstantPower    int
	LoadInstantPower    int
	BatteryInstantPower int
	SolarInstantPower   int
	BatteryCharge       float64
	QueryTime           time.Duration
}

// energyByLocation queries for the current information for a site.
func energyByLocation(locations ...string) ([]TopStats, error) {
	var allStats []TopStats = make([]TopStats, 0)

	for _, location := range locations {
		start := time.Now()
		var stats TopStats
		topic := "energy/" + location + "/energy"
		row := db.QueryRow("SELECT dt asof, battery_percent_full pct, payload->>'$.load.instant_power' ld, payload->>'$.battery.instant_power' battery, payload->>'$.site.instant_power' site, payload->>'$.solar.instant_power' solar FROM energy where topic = ? order by asOf desc limit 1;", topic)

		var (
			load    float64
			battery float64
			site    float64
			solar   float64
			pct     float64
		)
		if err := row.Scan(&stats.AsOf, &pct, &load, &battery, &site, &solar); err != nil {
			if err == sql.ErrNoRows {
				return allStats, fmt.Errorf("topic %s: no last row", topic)
			}
			s := fmt.Sprintf("%+v", err)
			return allStats, fmt.Errorf("energyByLocation() %s", s)
		}

		timeLoc, _ := time.LoadLocation("Local")
		stats.Location = strings.ToUpper(location)
		stats.AsOf = stats.AsOf.In(timeLoc)
		stats.QueryTime = time.Since(start)
		stats.LoadInstantPower = int(load)
		stats.BatteryInstantPower = int(battery)
		stats.SiteInstantPower = int(site)
		stats.SolarInstantPower = int(solar)
		stats.BatteryCharge = pct
		allStats = append(allStats, stats)
	}
	return allStats, nil
}

func main() {
	debugFlag := flag.Bool("debug", false, "sets log level to debugFlag")
	consoleFlag := flag.Bool("console", false, "directs output to stdout on the consoleFlag")

	flag.Parse()

	if *consoleFlag {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	} else {
		log.Output(os.Stdout)
	}

	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack
	err := errors.New("some error")
	stack := string(debug.Stack())
	log.Debug().Msg(stack)
	log.Error().Stack().Err(err).Msg("foo")
	// debug.PrintStack()
	log.Error().Err(err).Stack().Msg("enabling debugFlag level logging")
	log.Warn().Str("foo", string(3)).Msg("this is a warning")
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if *debugFlag {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		err := errors.New("some error")
		stack := string(debug.Stack())
		log.Debug().Msg(stack)
		// debug.PrintStack()
		log.Error().Stack().Err(err).Msg("enabling debugFlag level logging")
		log.Warn().Str("foo", string(3)).Msg("this is a warning")
	} else {
		log.Info().Msg("info level logging enabled")
	}
	log.Debug().Msg("about to call dbConnect()")
	dbConnect()
	log.Info().Msg("done calling dbConnect()")

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
	err = http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatal().Err(err).Msg("http.ListenAndServe()")
	}
}

func energyHandler(w http.ResponseWriter, r *http.Request) {
	stats := make([]TopStats, 0)
	stats, err := energyByLocation("ma", "vt")
	if err != nil {
		s := fmt.Sprintf("%+v", err)
		http.Error(w, s, http.StatusInternalServerError)
	}

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
