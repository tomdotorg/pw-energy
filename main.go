package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/rs/zerolog/pkgerrors"
)

var db *sql.DB = nil

// templateData provides template parameters.
type templateData struct {
	Service      string
	Revision     string
	Stats        TopStats
	LoadData     string
	SiteData     string
	BatteryData  string
	SolarData    string
	MQTTSubTopic string
	LiveLimit    int
	Location     string
}

type PctDisplayRecord struct {
	location       string
	topic          string
	dt             time.Time
	percentCharged float64
}

type TopStats struct {
	Location              string
	AsOf                  time.Time
	SiteInstantPower      int
	LoadInstantPower      int
	BatteryInstantPower   int
	SolarInstantPower     int
	BatteryCharge         float64
	BatteryChargeAsOf     time.Time
	QueryTime             time.Duration
	DayBatteryHistory     []BatteryPctDisplayRecord
	FiveMinBatteryHistory []BatteryPctDisplayRecord
	StatsHistory          []StatsDisplayRecord
	EnergyHistory         []StatsDisplayRecord
	ConsumedGraphData     string
	ProducedGraphData     string
	BatteryGraphData      string
	SiteGraphData         string
	BatteryPctGraphData   string
}

type BatteryPctDisplayRecord struct {
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

type EnergyDisplayRecord struct { // FIXME: what is the idiom for this pattern?
	AsOf     time.Time
	Location string
	Site     float64
	Load     float64
	Battery  float64
	Solar    float64
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
	SiteImported        float64
	SiteExported        float64
	SiteNet             float64
	NumSiteSamples      int
	TotalSiteSamples    float64
	SiteAvg             float64
	HiLoad              float64
	HiLoadTime          int64
	HiLoadDT            string
	LowLoad             float64
	LowLoadTime         int64
	LowLoadDT           string
	LoadImported        float64
	LoadExported        float64
	LoadNet             float64
	NumLoadSamples      int
	TotalLoadSamples    float64
	LoadAvg             float64
	HiBattery           float64
	HiBatteryTime       int64
	HiBatteryDT         string
	LowBattery          float64
	LowBatteryTime      int64
	LowBatteryDT        string
	BatteryImported     float64
	BatteryExported     float64
	BatteryNet          float64
	NumBatterySamples   int
	TotalBatterySamples float64
	BatteryAvg          float64
	HiSolar             float64
	HiSolarTime         int64
	HiSolarDT           string
	LowSolar            float64
	LowSolarTime        int64
	LowSolarDT          string
	SolarImported       float64
	SolarExported       float64
	SolarNet            float64
	NumSolarSamples     int
	TotalSolarSamples   float64
	SolarAvg            float64
}

// Variables used to generate the HTML page.
var (
	indexData     templateData
	indexTmpl     *template.Template
	dashboardTmpl *template.Template
	chartsTmpl    *template.Template
	liveTmpl      *template.Template
	instantTmpl   *template.Template
	liveData      templateData
)

type ValueDisplayRecord struct {
	DT    int64
	Value float64
}

func initLogs() {
	// initialize the logger
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	if os.Getenv("CONSOLE") != "" {
		log.Info().Msg("logging to console")
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	} else {
		log.Output(os.Stdout)
	}

	if os.Getenv("DEBUG") != "" {
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
		log.Info().Msg("enabling Trace level logging")
	} else {
		log.Info().Msg("enabling Info level logging")
	}
}

func dbConnect() {
	dbName := os.Getenv("DB_NAME")
	user := os.Getenv("DB_USER")
	passwd := os.Getenv("DB_PASS")
	instanceConnectionName := os.Getenv("INSTANCE_CONNECTION_NAME")
	socketDir, socketIsSet := os.LookupEnv("DB_SOCKET_DIR")
	// log.Debug("env", "DB_NAME", dbName).Msg("")
	if !socketIsSet {
		socketDir = "/cloudsql"
	}

	dbURI := fmt.Sprintf("%s:%s@unix(%s/%s)/%s?parseTime=true",
		user, passwd, socketDir, instanceConnectionName, dbName)
	debugURI := fmt.Sprintf("%s:%s@unix(%s/%s)/%s?parseTime=true",
		user, "<password>", socketDir, instanceConnectionName, dbName)
	log.Trace().Msgf("connecting to: [%s]", debugURI)
	// dbPool is the pool of database connections.
	var err error
	const retries = 5
	if db != nil {
		pingErr := db.Ping()
		if pingErr == nil {
			log.Print("Already connected.")
			return
		}
	}
	for i := 0; i < retries; i++ {
		db, err = sql.Open("mysql", dbURI)
		if err != nil {
			log.Fatal().Err(err).Msg("sql.Open()")
		}
		pingErr := db.Ping()
		if pingErr != nil {
			log.Error().Err(pingErr).Stack().Msgf("pinging db - try #%d", i)
		} else {
			log.Info().Msg("Connected!")
			return
		}
	}
	log.Fatal().Err(err).Msgf("Could not connect to database: %s", err)
}

func statsChartData(in []StatsDisplayRecord) (production string, consumption string, grid string, battery string) {
	log.Debug().Msg("statsChartData()")
	var prod, cons, site, batt strings.Builder

	prod.WriteString("[")
	cons.WriteString("[")
	site.WriteString("[")
	batt.WriteString("[")
	n := 0
	for _, v := range in {
		dt := v.DateTime * 1000
		prod.WriteString(fmt.Sprintf("[%d,%f],", dt, v.SolarAvg))
		cons.WriteString(fmt.Sprintf("[%d,%f],", dt, v.LoadAvg))
		site.WriteString(fmt.Sprintf("[%d,%f],", dt, v.SiteAvg))
		batt.WriteString(fmt.Sprintf("[%d,%f],", dt, v.BatteryAvg))
		n++
	}
	prod.WriteString("]")
	cons.WriteString("]")
	site.WriteString("]")
	batt.WriteString("]")

	log.Debug().Msgf("statsChartData() done: %d rows processed", n)
	return prod.String(), cons.String(), site.String(), batt.String()
}

func liveChartData(in []EnergyDisplayRecord) (p string, c string, s string, b string) {
	var prod, cons, site, batt strings.Builder

	prod.WriteString("[")
	cons.WriteString("[")
	site.WriteString("[")
	batt.WriteString("[")

	n := 0
	for _, v := range in {
		dt := v.AsOf.Local().Unix() * 1000
		cons.WriteString(fmt.Sprintf("[%d,%f],", dt, v.Load))
		site.WriteString(fmt.Sprintf("[%d,%f],", dt, v.Site))
		batt.WriteString(fmt.Sprintf("[%d,%f],", dt, v.Battery))
		prod.WriteString(fmt.Sprintf("[%d,%f],", dt, v.Solar))
	}

	prod.WriteString("]")
	cons.WriteString("]")
	site.WriteString("]")
	batt.WriteString("]")

	log.Debug().Msgf("liveChartData() done: %d rows processed", n)
	return prod.String(), cons.String(), site.String(), batt.String()
}

func batteryChartData(in []BatteryPctDisplayRecord) (p string) {
	var pct strings.Builder
	pct.WriteString("[")
	for _, v := range in {
		dt := v.DateTime * 1000
		pct.WriteString(fmt.Sprintf("[%d,%f],", dt, v.AvgPct))
	}
	pct.WriteString("]")
	return pct.String()
}

// statsByLocation queries for the summary information for a site.
func statsByLocation(location string, limit int) (TopStats, error) {
	log.Debug().Msgf("statsByLocation(%s, %d)", location, limit)
	dbConnect()
	start := time.Now()
	var stats TopStats
	row := db.QueryRow("SELECT dt asof, payload->>'$.load.instant_power' ld, payload->>'$.battery.instant_power' battery, payload->>'$.site.instant_power' site, payload->>'$.solar.instant_power' solar FROM energy where location = ? order by asOf desc limit 1;", location)
	var load, battery, site, solar float64
	if err := row.Scan(&stats.AsOf, &load, &battery, &site, &solar); err != nil {
		if err == sql.ErrNoRows {
			log.Error().Err(err).Msg("No rows returned")
			return stats, err
		}
		log.Error().Err(err).Msg("No rows returned")
		return stats, err
	}
	row = db.QueryRow("SELECT dt asof, percent_charged FROM battery where location = ? order by asOf desc limit 1;", location)
	if err := row.Scan(&stats.BatteryChargeAsOf, &stats.BatteryCharge); err != nil {
		if err == sql.ErrNoRows {
			log.Error().Err(err).Msg("no battery charge data")
			return stats, err
		}
		log.Error().Err(err).Msg("no battery charge data")
		return stats, err
	}

	timeLoc, _ := time.LoadLocation("Local")
	stats.Location = strings.ToUpper(location)
	stats.AsOf = stats.AsOf.In(timeLoc)
	stats.LoadInstantPower = int(load)
	stats.BatteryInstantPower = int(battery)
	stats.SiteInstantPower = int(site)
	stats.SolarInstantPower = int(solar)
	stats.BatteryChargeAsOf = stats.BatteryChargeAsOf.In(timeLoc)

	// Battery percent history
	battHistory, err := getDayBatteryPct(location, limit)
	if err != nil {
		log.Error().Err(err).Msg("getDayBatteryPct()")
	}
	stats.DayBatteryHistory = battHistory

	// Stats history
	statsHistory, err := getDayStats(location, limit)
	if err != nil {
		log.Error().Err(err).Msg("getDayBatteryPct()")
	}
	stats.StatsHistory = statsHistory

	stats.QueryTime = time.Since(start)
	return stats, nil
}

// currentEnergyByLocation returns the limit most current records
func currentEnergyByLocation(location string, limit int) ([]EnergyDisplayRecord, error) {
	log.Debug().Msgf("currentEnergyByLocation(%s, %d)", location, limit)
	dbConnect()
	var energy EnergyDisplayRecord
	var energyList = make([]EnergyDisplayRecord, 0)
	row, err := db.Query("select * from (SELECT id, dt asof, load_instant_power ld, battery_instant_power battery, site_instant_power site, solar_instant_power solar FROM energy where location = ? order by asOf desc limit ?) t1 order by t1.id;", location, limit)
	if err != nil {
		log.Error().Err(err).Msg("currentEnergyByLocation()")
		return energyList, err
	}
	for row.Next() {
		var id int
		err := row.Scan(&id, &energy.AsOf, &energy.Load, &energy.Battery, &energy.Site, &energy.Solar)
		if err != nil {
			if err == sql.ErrNoRows {
				log.Error().Err(err).Msg("No rows returned")
				return energyList, err
			}
			log.Error().Err(err).Msg("No rows returned")
			return energyList, err
		}
		energy.Location = location
		timeLoc, _ := time.LoadLocation("Local")
		energy.AsOf = energy.AsOf.In(timeLoc)
		energyList = append(energyList, energy)
	}
	return energyList, nil
}

func main() {
	initLogs()
	log.Debug().Msg("about to call dbConnect()")
	dbConnect()
	log.Debug().Msg("done calling dbConnect()")

	http.HandleFunc("/", indexHandler)

	/* flare students work */

	// Prepare template for execution.
	instantTmpl = template.Must(template.ParseFiles("instant.html"))
	liveData = templateData{
		Service:  "instant service",
		Revision: "0.1",
	}
	http.HandleFunc("/instant", instantHandler)

	// Prepare template for execution.
	liveTmpl = template.Must(template.ParseFiles("live.html"))
	liveData = templateData{
		Service:  "live service",
		Revision: "0.1",
	}
	http.HandleFunc("/live", liveHandler)
	dashboardTmpl = template.Must(template.ParseFiles("dashboard.html"))

	http.HandleFunc("/energy", energyHandler)

	fs := http.FileServer(http.Dir("./assets"))
	http.Handle("/assets/", http.StripPrefix("/assets/", fs))

	// PORT environment variable is provided by Cloud Run.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Info().Msgf("Listening on port %s", port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatal().Err(err).Msg("http.ListenAndServe()")
	}
}

func energyHandler(w http.ResponseWriter, r *http.Request) {
	var stats TopStats
	var location string
	keys, ok := r.URL.Query()["location"]
	if !ok || len(keys) != 1 {
		location = "VT"
	} else {
		location = strings.ToUpper(keys[0])
	}
	log.Debug().Msgf(`location: %s`, location)

	defaultLimit, err := strconv.Atoi(os.Getenv("DEFAULT_LIMIT"))
	if err != nil {
		log.Error().Err(err).Msg("DEFAULT_LIMIT failed strconv.Atoi()")
		defaultLimit = 7
	}
	var limit int

	limits, ok := r.URL.Query()["limit"]
	if !ok || len(limits[0]) < 1 {
		limit = defaultLimit
	} else {
		l, err := strconv.Atoi(limits[0])
		if err != nil {
			log.Warn().Msgf("limit [%s] not an integer - using %d", keys[0], defaultLimit)
			limit = defaultLimit
		} else {
			limit = l
		}
	}

	stats, err = statsByLocation(location, limit)
	if err != nil {
		s := fmt.Sprintf("%+v", err)
		http.Error(w, s, http.StatusInternalServerError)
	}

	const graphDays = 60
	beginDate := time.Now().Local().AddDate(0, 0, -1*graphDays).Unix()
	endDate := time.Now().Local().Unix()
	fiveMinStatRecs, err := getFiveMinStats(location, beginDate, endDate)
	if err != nil {
		log.Error().Err(err).Msg("getFiveMinStats()")
	}
	stats.EnergyHistory = fiveMinStatRecs
	stats.ProducedGraphData, stats.ConsumedGraphData, stats.SiteGraphData, stats.BatteryGraphData = statsChartData(fiveMinStatRecs)

	fiveMinBatteryRecs, err := getFiveMinBattery(location, beginDate, endDate)
	if err != nil {
		log.Error().Stack().Err(err).Msg("getFiveMinBattery()")
	}
	stats.FiveMinBatteryHistory = fiveMinBatteryRecs
	stats.BatteryPctGraphData = batteryChartData(fiveMinBatteryRecs)

	if err := dashboardTmpl.Execute(w, stats); err != nil {
		msg := http.StatusText(http.StatusInternalServerError)
		log.Error().Err(err).Msg(msg)
	}
}

// indexHandler responds by redirecting to Google search.
func indexHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "https://www.google.com", 301)
}

func liveHandler(w http.ResponseWriter, r *http.Request) {
	var location string
	keys, ok := r.URL.Query()["location"]
	if !ok || len(keys) != 1 {
		log.Debug().Msgf(`no location specified in location url parameter. using VT`)
		location = "VT"
	} else {
		location = strings.ToUpper(keys[0])
	}
	log.Debug().Msgf(`location: %s`, location)

	limit, ok := r.URL.Query()["limit"]
	if !ok || len(limit) != 1 {
		log.Debug().Msgf(`no limit specified in limit url parameter. using 2000`)
		liveData.LiveLimit = 2000
	} else {
		var err error
		if liveData.LiveLimit, err = strconv.Atoi(limit[0]); err != nil {
			liveData.LiveLimit = 2000
			log.Warn().Msgf("limit [%s] not an integer - using %d", limit[0], liveData.LiveLimit)
		}
	}
	log.Debug().Msgf(`LiveLimit: %d`, liveData.LiveLimit)

	recs, err := currentEnergyByLocation(location, liveData.LiveLimit)
	if err != nil {
		s := fmt.Sprintf("%+v", err)
		http.Error(w, s, http.StatusInternalServerError)
	}
	log.Debug().Msgf("live recs: %+v", len(recs))
	liveData.MQTTSubTopic = "energy/" + strings.ToLower(location) + "/energy" // works with wildcard # and + topics dynamically now
	log.Debug().Msgf(`liveData.MQTTSubTopic: %s`, liveData.MQTTSubTopic)

	liveData.Location = location
	liveData.SolarData, liveData.LoadData, liveData.SiteData, liveData.BatteryData = liveChartData(recs)
	if err := liveTmpl.Execute(w, liveData); err != nil {
		msg := http.StatusText(http.StatusInternalServerError)
		log.Error().Err(err).Stack().Msg(msg)
	}
}

func instantHandler(w http.ResponseWriter, r *http.Request) {
	//var location string
	//keys, ok := r.URL.Query()["location"]
	//if !ok || len(keys) != 1 {
	//	log.Debug().Msgf(`no location specified in location url parameter. using VT`)
	//	location = "VT"
	//} else {
	//	location = strings.ToUpper(keys[0])
	//}
	//log.Debug().Msgf(`location: %s`, location)
	//
	//limit, ok := r.URL.Query()["limit"]
	//if !ok || len(limit) != 1 {
	//	log.Debug().Msgf(`no limit specified in limit url parameter. using 2000`)
	//	liveData.LiveLimit = 2000
	//} else {
	//	var err error
	//	if liveData.LiveLimit, err = strconv.Atoi(limit[0]); err != nil {
	//		liveData.LiveLimit = 2000
	//		log.Warn().Msgf("limit [%s] not an integer - using %d", limit[0], liveData.LiveLimit)
	//	}
	//}
	//log.Debug().Msgf(`LiveLimit: %d`, liveData.LiveLimit)
}

func getDayStats(location string, limit int) ([]StatsDisplayRecord, error) {
	log.Debug().Msgf("getDayStats(%s, %d)", location, limit)
	rows, err := db.Query(`select location, datetime,
       hi_site, hi_site_dt, low_site, low_site_dt, site_energy_imported, site_energy_exported, num_site_samples, total_site_samples,
		   hi_load, hi_load_dt, low_load, low_load_dt, load_energy_imported, load_energy_exported, num_load_samples, total_load_samples,
		   hi_battery, hi_battery_dt, low_battery, low_battery_dt, battery_energy_imported, battery_energy_exported, num_battery_samples, total_battery_samples,
		   hi_solar, hi_solar_dt, low_solar, low_solar_dt, solar_energy_imported, solar_energy_exported, num_solar_samples, total_solar_samples
			from day_top_stats where location = ? order by datetime desc limit ?`, location, limit)
	if err != nil {
		log.Error().Err(err).Msgf("getDayStats(): %+v", err)
		return nil, err
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			log.Fatal().Err(err).Stack().Msg("error closing rows")
		}
	}(rows)
	recs := make([]StatsDisplayRecord, 0)

	for i := 0; i < limit && rows.Next(); i++ {
		var dbStats StatsDisplayRecord
		err = rows.Scan(&dbStats.Location, &dbStats.DateTime,
			&dbStats.HiSite, &dbStats.HiSiteTime, &dbStats.LowSite, &dbStats.LowSiteTime, &dbStats.SiteImported, &dbStats.SiteExported, &dbStats.NumSiteSamples, &dbStats.TotalSiteSamples,
			&dbStats.HiLoad, &dbStats.HiLoadTime, &dbStats.LowLoad, &dbStats.LowLoadTime, &dbStats.LoadImported, &dbStats.LoadExported, &dbStats.NumLoadSamples, &dbStats.TotalLoadSamples,
			&dbStats.HiBattery, &dbStats.HiBatteryTime, &dbStats.LowBattery, &dbStats.LowBatteryTime, &dbStats.BatteryImported, &dbStats.BatteryExported, &dbStats.NumBatterySamples, &dbStats.TotalBatterySamples,
			&dbStats.HiSolar, &dbStats.HiSolarTime, &dbStats.LowSolar, &dbStats.LowSolarTime, &dbStats.SolarImported, &dbStats.SolarExported, &dbStats.NumSolarSamples, &dbStats.TotalSolarSamples)
		if err != nil {
			log.Error().Err(err).Msgf("getDayStats(): %+v", err)
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
	log.Debug().Msgf("end getDayStats()")
	return recs, nil
}

func getFiveMinStats(location string, beginDate int64, endDate int64) ([]StatsDisplayRecord, error) {
	log.Debug().Msgf("getFiveMinStats(%s, %d  %d)", location, beginDate, endDate)
	rows, err := db.Query(`select location, datetime,
       hi_site, hi_site_dt, low_site, low_site_dt, site_energy_imported, site_energy_exported, num_site_samples, total_site_samples,
		   hi_load, hi_load_dt, low_load, low_load_dt, load_energy_imported, load_energy_exported, num_load_samples, total_load_samples,
		   hi_battery, hi_battery_dt, low_battery, low_battery_dt, battery_energy_imported, battery_energy_exported, num_battery_samples, total_battery_samples,
		   hi_solar, hi_solar_dt, low_solar, low_solar_dt, solar_energy_imported, solar_energy_exported, num_solar_samples, total_solar_samples
			from five_min_top_stats where location = ? and datetime >= ? and datetime <= ? order by datetime`, location, beginDate, endDate)
	if err != nil {
		log.Error().Err(err).Msgf("getFiveMinStats(): %+v", err)
		return nil, err
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			log.Fatal().Err(err).Stack().Msg("error closing rows")
		}
	}(rows)
	recs := make([]StatsDisplayRecord, 0)

	for i := 0; rows.Next(); i++ {
		var dbStats StatsDisplayRecord
		err = rows.Scan(&dbStats.Location, &dbStats.DateTime,
			&dbStats.HiSite, &dbStats.HiSiteTime, &dbStats.LowSite, &dbStats.LowSiteTime, &dbStats.SiteImported, &dbStats.SiteExported, &dbStats.NumSiteSamples, &dbStats.TotalSiteSamples,
			&dbStats.HiLoad, &dbStats.HiLoadTime, &dbStats.LowLoad, &dbStats.LowLoadTime, &dbStats.LoadImported, &dbStats.LoadExported, &dbStats.NumLoadSamples, &dbStats.TotalLoadSamples,
			&dbStats.HiBattery, &dbStats.HiBatteryTime, &dbStats.LowBattery, &dbStats.LowBatteryTime, &dbStats.BatteryImported, &dbStats.BatteryExported, &dbStats.NumBatterySamples, &dbStats.TotalBatterySamples,
			&dbStats.HiSolar, &dbStats.HiSolarTime, &dbStats.LowSolar, &dbStats.LowSolarTime, &dbStats.SolarImported, &dbStats.SolarExported, &dbStats.NumSolarSamples, &dbStats.TotalSolarSamples)
		if err != nil {
			log.Error().Err(err).Msgf("getFiveMinStats(): %+v", err)
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
	log.Debug().Msgf("end getFiveMinStats()")
	return recs, nil
}

func getFiveMinBattery(location string, beginDate int64, endDate int64) ([]BatteryPctDisplayRecord, error) {
	log.Debug().Msgf("getFiveMinBattery(%s, %+v, %+v)", location, time.Unix(beginDate, 0).String(), time.Unix(endDate, 0).String())
	rows, err := db.Query("select location, datetime, hi_pct, hi_pct_dt, low_pct, low_pct_dt, "+
		"num_samples, total_samples from five_min_battery_pct where location = ? "+
		"and datetime >= ? and datetime <= ? order by datetime", location, beginDate, endDate)
	if err != nil {
		log.Error().Err(err).Stack().Msg("error querying db")
		return nil, err
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			log.Error().Err(err).Stack().Msg("error closing rows")
		}
	}(rows)
	recs := make([]BatteryPctDisplayRecord, 0)
	for rows.Next() {
		var pctRecord BatteryPctDisplayRecord
		err = rows.Scan(&pctRecord.Location, &pctRecord.DateTime, &pctRecord.HiPct, &pctRecord.HiPctTime, &pctRecord.LowPct, &pctRecord.LowPctTime, &pctRecord.NumSamples, &pctRecord.TotalSamples)
		if err != nil {
			log.Error().Err(err).Stack().Msg("error getting day pct summaries")
			return recs, err
		}
		pctRecord.DT = time.Unix(pctRecord.DateTime, 0).Format("2006-01-02")
		pctRecord.LowDT = time.Unix(pctRecord.LowPctTime, 0).Format("15:04")
		pctRecord.HiDT = time.Unix(pctRecord.HiPctTime, 0).Format("15:04")
		pctRecord.AvgPct = pctRecord.TotalSamples / float64(pctRecord.NumSamples)
		//		log.Debug().Msgf("pctRecord: %+v", pctRecord)
		recs = append(recs, pctRecord)
	}
	log.Debug().Msgf("end getFiveMinBattery() returning %d records", len(recs))
	return recs, nil
}

func getDayBatteryPct(location string, limit int) ([]BatteryPctDisplayRecord, error) {
	log.Debug().Msgf("getDayBatteryPct(%s, %d)", location, limit)
	rows, err := db.Query(
		"select location, datetime, hi_pct, hi_pct_dt, low_pct, low_pct_dt, "+
			"num_samples, total_samples from day_battery_pct where location = ? order by datetime desc limit ?",
		location, limit)
	if err != nil {
		log.Error().Err(err).Stack().Msg("error querying db")
		return nil, err
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			log.Error().Err(err).Stack().Msg("error closing rows")
		}
	}(rows)
	recs := make([]BatteryPctDisplayRecord, 0)
	for i := 0; i < limit && rows.Next(); i++ {
		var pctRecord BatteryPctDisplayRecord
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
	log.Debug().Msgf("end getDayBatteryPct(%s, %d)", location, limit)
	return recs, nil
}
