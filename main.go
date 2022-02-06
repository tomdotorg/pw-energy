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
	chartsTmpl    *template.Template
)

func init() {
	// initialize the logger
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	debugFlag := flag.Bool("debug", false, "sets log level to debugFlag")
	consoleFlag := flag.Bool("console", false, "directs output to stdout on the consoleFlag")

	flag.Parse()

	if *consoleFlag {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	} else {
		log.Output(os.Stdout)
	}

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
	pingErr := db.Ping()
	if pingErr != nil {
		log.Fatal().Err(pingErr).Stack().Msg("pinging db")
	}
	log.Print("Connected!")
}

type TopStats struct {
	Location            string
	AsOf                time.Time
	SiteInstantPower    int
	LoadInstantPower    int
	BatteryInstantPower int
	SolarInstantPower   int
	BatteryCharge       float64
	QueryTime           time.Duration
	PctHistory          []DayBatteryPctDisplayRecord
	StatsHistory        []StatsDisplayRecord
}

type DayBatteryPctDisplayRecord struct {
	Location     string
	DateTime     int64
	DT           string
	HiPct        float64
	HiPctTime    int64
	HiDT         string
	LowPct       float64
	LowPctTime   int64
	LowDT        string
	NumSamples   int
	TotalSamples float64
	AvgPct       float64
}

type StatsDisplayRecord struct {
	Location            string
	DateTime            int64
	DT                  string
	HiSite              float64
	HiSiteTime          int64
	HiSiteDT            string
	LowSite             float64
	LowSiteTime         int64
	LowSiteDT           string
	NumSiteSamples      int
	TotalSiteSamples    float64
	SiteAvg             float64
	HiLoad              float64
	HiLoadTime          int64
	HiLoadDT            string
	LowLoad             float64
	LowLoadTime         int64
	LowLoadDT           string
	NumLoadSamples      int
	TotalLoadSamples    float64
	LoadAvg             float64
	HiBattery           float64
	HiBatteryTime       int64
	HiBatteryDT         string
	LowBattery          float64
	LowBatteryTime      int64
	LowBatteryDT        string
	NumBatterySamples   int
	TotalBatterySamples float64
	BatteryAvg          float64
	HiSolar             float64
	HiSolarTime         int64
	HiSolarDT           string
	LowSolar            float64
	LowSolarTime        int64
	LowSolarDT          string
	NumSolarSamples     int
	TotalSolarSamples   float64
	SolarAvg            float64
}

type ChartData struct {
	Name       string
	Type       string
	NumAxis    int
	AxisLabels []string
	AxisData   []float64
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

		// Battery percent history
		battHistory, err := getPct(location)
		if err != nil {
			log.Error().Err(err).Msg("getPct()")
		}
		stats.PctHistory = battHistory

		// Stats history
		statsHistory, err := getStats(location)
		if err != nil {
			log.Error().Err(err).Msg("getPct()")
		}
		stats.StatsHistory = statsHistory
		log.Debug().Msgf("statsHistory: %+v", statsHistory)
		allStats = append(allStats, stats)
	}
	return allStats, nil
}

func main() {
	log.Debug().Msg("about to call dbConnect()")
	dbConnect()
	log.Info().Msg("done calling dbConnect()")

	// Prepare template for execution.
	indexTmpl = template.Must(template.ParseFiles("index.html"))
	indexData = templateData{
		Service:  "sample service",
		Revision: "1.0",
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
		log.Error().Err(err).Msg(msg)
	}

}

// helloRunHandler responds by rendering an HTML page.
func helloRunHandler(w http.ResponseWriter, r *http.Request) {
	if err := indexTmpl.Execute(w, indexData); err != nil {
		msg := http.StatusText(http.StatusInternalServerError)
		log.Printf("template.Execute: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
	}
}

func getStats(location string) ([]StatsDisplayRecord, error) {
	rows, err := db.Query(`select location, datetime,
       hi_site, hi_site_dt, low_site, low_site_dt, num_site_samples, total_site_samples,
		   hi_load, hi_load_dt, low_load, low_load_dt, num_load_samples, total_load_samples,
		   hi_battery, hi_battery_dt, low_battery, low_battery_dt, num_battery_samples, total_battery_samples,
		   hi_solar, hi_solar_dt, low_solar, low_solar_dt, num_solar_samples, total_solar_samples
			from day_top_stats where location = ? order by datetime desc`, location)
	if err != nil {
		log.Error().Err(err).Msgf("getStats(): %+v", err)
		return nil, err
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			log.Fatal().Err(err).Stack().Msg("error closing rows")
		}
	}(rows)
	recs := make([]StatsDisplayRecord, 0)

	for rows.Next() {
		var dbStats StatsDisplayRecord
		err = rows.Scan(&dbStats.Location, &dbStats.DateTime,
			&dbStats.HiSite, &dbStats.HiSiteTime, &dbStats.LowSite, &dbStats.LowSiteTime, &dbStats.NumSiteSamples, &dbStats.TotalSiteSamples,
			&dbStats.HiLoad, &dbStats.HiLoadTime, &dbStats.LowLoad, &dbStats.LowLoadTime, &dbStats.NumLoadSamples, &dbStats.TotalLoadSamples,
			&dbStats.HiBattery, &dbStats.HiBatteryTime, &dbStats.LowBattery, &dbStats.LowBatteryTime, &dbStats.NumBatterySamples, &dbStats.TotalBatterySamples,
			&dbStats.HiSolar, &dbStats.HiSolarTime, &dbStats.LowSolar, &dbStats.LowSolarTime, &dbStats.NumSolarSamples, &dbStats.TotalSolarSamples)
		if err != nil {
			log.Error().Err(err).Msgf("getStats(): %+v", err)
			return nil, err
		}
		dbStats.DT = time.Unix(dbStats.DateTime, 0).Format("2006-01-02")
		dbStats.LowSiteDT = time.Unix(dbStats.LowSiteTime, 0).Format("15:04")
		dbStats.HiSiteDT = time.Unix(dbStats.HiSiteTime, 0).Format("15:04")
		dbStats.SiteAvg = dbStats.TotalSiteSamples / float64(dbStats.NumSiteSamples)
		dbStats.LowBatteryDT = time.Unix(dbStats.LowBatteryTime, 0).Format("15:04")
		dbStats.HiBatteryDT = time.Unix(dbStats.HiBatteryTime, 0).Format("15:04")
		dbStats.BatteryAvg = dbStats.TotalBatterySamples / float64(dbStats.NumBatterySamples)
		dbStats.LowLoadDT = time.Unix(dbStats.LowLoadTime, 0).Format("15:04")
		dbStats.HiLoadDT = time.Unix(dbStats.HiLoadTime, 0).Format("15:04")
		dbStats.LoadAvg = dbStats.TotalLoadSamples / float64(dbStats.NumLoadSamples)
		dbStats.LowSolarDT = time.Unix(dbStats.LowSolarTime, 0).Format("15:04")
		dbStats.HiSolarDT = time.Unix(dbStats.HiSolarTime, 0).Format("15:04")
		dbStats.SolarAvg = dbStats.TotalSolarSamples / float64(dbStats.NumSolarSamples)
		recs = append(recs, dbStats)
	}
	return recs, nil
}

func getPct(location string) ([]DayBatteryPctDisplayRecord, error) {
	rows, err := db.Query(
		"select location, datetime, hi_pct, hi_pct_dt, low_pct, low_pct_dt, "+
			"num_samples, total_samples from day_battery_pct where location = ? order by datetime desc",
		location,
	)
	if err != nil {
		log.Error().Err(err).Stack().Msg("error querying for update")
		return nil, err
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			log.Error().Err(err).Stack().Msg("error closing rows")
		}
	}(rows)
	recs := make([]DayBatteryPctDisplayRecord, 0)
	for rows.Next() {
		var pctRecord DayBatteryPctDisplayRecord
		err = rows.Scan(&pctRecord.Location, &pctRecord.DateTime, &pctRecord.HiPct, &pctRecord.HiPctTime, &pctRecord.LowPct, &pctRecord.LowPctTime, &pctRecord.NumSamples, &pctRecord.TotalSamples)
		if err != nil {
			log.Error().Err(err).Stack().Msg("error getting day pct summaries")
			return recs, err
		}
		pctRecord.DT = time.Unix(pctRecord.DateTime, 0).Format("2006-01-02")
		pctRecord.LowDT = time.Unix(pctRecord.LowPctTime, 0).Format("15:04")
		pctRecord.HiDT = time.Unix(pctRecord.HiPctTime, 0).Format("15:04")
		pctRecord.AvgPct = pctRecord.TotalSamples / float64(pctRecord.NumSamples)
		recs = append(recs, pctRecord)
	}
	return recs, nil
}
