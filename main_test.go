package main

import (
	"math/rand"
	"testing"
	"text/template"
	"time"

	"github.com/rs/zerolog/log"
)

func testInit() {
	initLogs()
}

func TestParseLive(t *testing.T) {
	testInit()
	liveTmpl = template.Must(template.ParseFiles("live.html"))
}

func TestLiveChartData(t *testing.T) {
	testInit()
	// Location string
	// Site     float64
	// Load     float64
	// Battery  float64
	// Solar    float64

	var testcases []EnergyDisplayRecord

	asOf := time.Now()
	for i := 0; i < 10; i++ {
		asOf = asOf.Add(time.Second * 3 * -1)
		rec := EnergyDisplayRecord{Site: 1000, Load: 1000, Battery: 1000, Solar: 1000, AsOf: asOf}
		testcases = append(testcases, rec)
		log.Printf("testcase #%d: %v", i, testcases[i])
	}
	// func liveChartData(in []EnergyDisplayRecord) (prod string, cons string, site string, batt string) {

}

func TestFoo(t *testing.T) {
	testInit()
	rnd := rand.Rand{}
	t.Log("foo", rnd)
}

func TestBar(t *testing.T) {
	testInit()
	t.Log("bar")
}

// func TestService(t *testing.T) {
// 	port := os.Getenv("PORT")
// 	if port == "" {
// 		port = "8080"
// 	}
//
// 	url := os.Getenv("SERVICE_URL")
// 	if url == "" {
// 		url = "http://localhost:" + port
// 	}
//
// 	retryClient := retry.NewClient()
// 	req, err := retry.NewRequest(http.MethodGet, url+"/", nil)
// 	if err != nil {
// 		t.Fatalf("retry.NewRequest: %v", err)
// 	}
//
// 	token := os.Getenv("TOKEN")
// 	if token != "" {
// 		req.Header.Set("Authorization", "Bearer "+token)
// 	}
//
// 	resp, err := retryClient.Do(req)
// 	if err != nil {
// 		t.Fatalf("retryClient.Do: %v", err)
// 	}
//
// 	if got := resp.StatusCode; got != http.StatusOK {
// 		t.Errorf("HTTP Response: got %q, want %q", got, http.StatusOK)
// 	}
//
// 	out, err := ioutil.ReadAll(resp.Body)
// 	if err != nil {
// 		t.Fatalf("ioutil.ReadAll: %v", err)
// 	}
//
// 	want := "Congratulations, you successfully deployed a container image to Cloud Run"
// 	if !strings.Contains(string(out), want) {
// 		t.Errorf("HTTP Response: body does not include %q", want)
// 	}
// }
