package main

import (
	"math/rand"
	"testing"
)

// func init() {
// 	fmt.Println("initLogs()")
// 	testDotV := flag.Bool("test.v", false, "test.v")
// 	testRun := flag.Bool("test.run", false, "test.run")
// 	testPanic := flag.Bool("test.paniconexit0", false, "test.paniconexit0")
// 	testLogFile := flag.Bool("test.testlogfile", false, "test.testlogfile")
// 	fmt.Println("testDotV", testDotV)
// 	fmt.Println("testPanic", testPanic)
// 	fmt.Println("testLogFile", testLogFile)
// 	fmt.Println("testRun", testRun)
// 	flag.Parse()
// 	testing.Init()
// }

func TestFoo(t *testing.T) {
	rnd := rand.Rand{}
	t.Log("foo", rnd)
}

func TestBar(t *testing.T) {
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
