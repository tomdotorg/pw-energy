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

	if err := os.Setenv("TZ", "America/New_York"); err != nil {
		log.Fatal().Str("TZ", os.Getenv("TZ")).Msg("setting env.. missing or invalid TZ")
	}
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
	// db, err = sql.Open("mysql", cfg.FormatDSN())
	// if err != nil {
	// 	log.Fatal(err)
	// }
	//
	pingErr := db.Ping()
	if pingErr != nil {
		log.Fatal().Err(pingErr).Stack().Msg("pinging db")
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
	Pct                 []DayBatteryPctDisplayRecord
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

type StatsRecord struct {
	Location            string
	DateTime            int64
	HiSite              float64
	HiSiteTime          int64
	LowSite             float64
	LowSiteTime         int64
	NumSiteSamples      int
	TotalSiteSamples    float64
	HiLoad              float64
	HiLoadTime          int64
	LowLoad             float64
	LowLoadTime         int64
	NumLoadSamples      int
	TotalLoadSamples    float64
	HiBattery           float64
	HiBatteryTime       int64
	LowBattery          float64
	LowBatteryTime      int64
	NumBatterySamples   int
	TotalBatterySamples float64
	HiSolar             float64
	HiSolarTime         int64
	LowSolar            float64
	LowSolarTime        int64
	NumSolarSamples     int
	TotalSolarSamples   float64
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

		history, err := getPct(location)
		if err != nil {
			log.Error().Err(err).Msg("getPct()")
		}
		stats.Pct = history
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

// helloRunHandler responds to requests by rendering an HTML page.
func helloRunHandler(w http.ResponseWriter, r *http.Request) {
	if err := indexTmpl.Execute(w, indexData); err != nil {
		msg := http.StatusText(http.StatusInternalServerError)
		log.Printf("template.Execute: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
	}
}

/*
func getStats(msg mqtt.Message) error {
	location := topics[msg.Topic()]
	var soc SocRecord
	err := json.Unmarshal(msg.Payload(), &soc)
	if err != nil {
		return fmt.Errorf("addStats(): %+v", err)
	}
	log.Debug().Msgf("addStats(): %+v", soc)
	dt := getMidnightInUnix(soc.AsOf)
	rows, err := db.Query(`select location, datetime,
       hi_site, hi_site_dt, low_site, low_site_dt, num_site_samples, total_site_samples,
		   hi_load, hi_load_dt, low_load, low_load_dt, num_load_samples, total_load_samples,
		   hi_battery, hi_battery_dt, low_battery, low_battery_dt, num_battery_samples, total_battery_samples,
		   hi_solar, hi_solar_dt, low_solar, low_solar_dt, num_solar_samples, total_solar_samples
			from day_top_stats where location = ? and datetime = ? for update`, location, dt)
	if err != nil {
		return fmt.Errorf("addStats(): %+v", err)
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			log.Fatal().Err(err).Stack().Msg("error closing rows")
		}
	}(rows)
	if rows.Next() {
		var dbStats StatsRecord
		err = rows.Scan(&dbStats.Location, &dbStats.DateTime,
			&dbStats.HiSite, &dbStats.HiSiteTime, &dbStats.LowSite, &dbStats.LowSiteTime, &dbStats.NumSiteSamples, &dbStats.TotalSiteSamples,
			&dbStats.HiLoad, &dbStats.HiLoadTime, &dbStats.LowLoad, &dbStats.LowLoadTime, &dbStats.NumLoadSamples, &dbStats.TotalLoadSamples,
			&dbStats.HiBattery, &dbStats.HiBatteryTime, &dbStats.LowBattery, &dbStats.LowBatteryTime, &dbStats.NumBatterySamples, &dbStats.TotalBatterySamples,
			&dbStats.HiSolar, &dbStats.HiSolarTime, &dbStats.LowSolar, &dbStats.LowSolarTime, &dbStats.NumSolarSamples, &dbStats.TotalSolarSamples)
		if err != nil {
			return fmt.Errorf("addStats(): %+v", err)
		}
		_, err = db.Exec(`update day_top_stats set
                         num_site_samples = num_site_samples + 1, total_site_samples = total_site_samples + ?,
                         num_load_samples = num_load_samples + 1, total_load_samples = total_load_samples + ?,
                         num_battery_samples = num_battery_samples + 1, total_battery_samples = total_battery_samples + ?,
                         num_solar_samples = num_solar_samples + 1, total_solar_samples = total_solar_samples + ?
                         where datetime = ? and location = ?`, soc.Site.InstantPower, soc.Load.InstantPower,
			soc.Battery.InstantPower, soc.Solar.InstantPower, dt, location)
		if err != nil {
			return fmt.Errorf("addStats(): %+v", err)
		}

		if soc.Site.InstantPower > dbStats.HiSite {
			_, err = db.Exec("update day_top_stats set hi_site = ?, hi_site_dt = ? where datetime = ? and location = ?",
				soc.Site.InstantPower, soc.AsOf.Unix(), dt, location)
		}
		if err != nil {
			return fmt.Errorf("addStats(): %+v", err)
		}
		if soc.Site.InstantPower < dbStats.LowSite {
			_, err = db.Exec("update day_top_stats set low_site = ?, low_site_dt = ? where datetime = ? and location = ?",
				soc.Site.InstantPower, soc.AsOf.Unix(), dt, location)
		}
		if err != nil {
			return fmt.Errorf("addStats(): %+v", err)
		}
		if soc.Load.InstantPower > dbStats.HiLoad {
			_, err = db.Exec("update day_top_stats set hi_load = ?, hi_load_dt = ? where datetime = ? and location = ?",
				soc.Load.InstantPower, soc.AsOf.Unix(), dt, location)
		}
		if err != nil {
			return fmt.Errorf("addStats(): %+v", err)
		}
		if soc.Load.InstantPower < dbStats.LowLoad {
			_, err = db.Exec("update day_top_stats set low_load = ?, low_load_dt = ? where datetime = ? and location = ?",
				soc.Load.InstantPower, soc.AsOf.Unix(), dt, location)
		}
		if err != nil {
			return fmt.Errorf("addStats(): %+v", err)
		}
		if soc.Battery.InstantPower > dbStats.HiBattery {
			_, err = db.Exec("update day_top_stats set hi_battery = ?, hi_battery_dt = ? where datetime = ? and location = ?",
				soc.Battery.InstantPower, soc.AsOf.Unix(), dt, location)
		}
		if err != nil {
			return fmt.Errorf("addStats(): %+v", err)
		}
		if soc.Battery.InstantPower < dbStats.LowBattery {
			_, err = db.Exec("update day_top_stats set low_battery = ?, low_battery_dt = ? where datetime = ? and location = ?",
				soc.Battery.InstantPower, soc.AsOf.Unix(), dt, location)
		}
		if err != nil {
			return fmt.Errorf("addStats(): %+v", err)
		}
		if soc.Solar.InstantPower > dbStats.HiSolar {
			_, err = db.Exec("update day_top_stats set hi_solar = ?, hi_solar_dt = ? where datetime = ? and location = ?",
				soc.Solar.InstantPower, soc.AsOf.Unix(), dt, location)
		}
		if err != nil {
			return fmt.Errorf("addStats(): %+v", err)
		}
		if soc.Solar.InstantPower < dbStats.LowSolar {
			_, err = db.Exec("update day_top_stats set low_solar = ?, low_solar_dt = ? where datetime = ? and location = ?",
				soc.Solar.InstantPower, soc.AsOf.Unix(), dt, location)
		}
		if err != nil {
			return fmt.Errorf("addStats(): %+v", err)
		}
	} else {
		_, err = db.Exec("insert into day_top_stats (datetime, location, hi_site, hi_site_dt, low_site, low_site_dt, num_site_samples, total_site_samples, hi_load, hi_load_dt, low_load, low_load_dt, num_load_samples, total_load_samples, hi_battery, hi_battery_dt, low_battery, low_battery_dt, num_battery_samples, total_battery_samples, hi_solar, hi_solar_dt, low_solar, low_solar_dt, num_solar_samples, total_solar_samples) values (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
			dt, location, soc.Site.InstantPower, soc.AsOf.Unix(), soc.Site.InstantPower, soc.AsOf.Unix(), 1, soc.Site.InstantPower,
			soc.Load.InstantPower, soc.AsOf.Unix(), soc.Load.InstantPower, soc.AsOf.Unix(), 1, soc.Load.InstantPower,
			soc.Battery.InstantPower, soc.AsOf.Unix(), soc.Battery.InstantPower, soc.AsOf.Unix(), 1, soc.Battery.InstantPower,
			soc.Solar.InstantPower, soc.AsOf.Unix(), soc.Solar.InstantPower, soc.AsOf.Unix(), 1, soc.Solar.InstantPower)
		if err != nil {
			return fmt.Errorf("addStats(): %+v", err)
		}
	}
	return nil
}
*/

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
	return recs, err
}
