package apiserver

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/crowdsecurity/crowdsec/pkg/models"
	"github.com/gin-gonic/gin"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func InitMachineTest() (*gin.Engine, models.WatcherAuthResponse, error) {
	router, err := NewAPITest()
	if err != nil {
		return nil, models.WatcherAuthResponse{}, fmt.Errorf("unable to run local API: %s", err)
	}

	body, err := CreateTestMachine(router)
	if err != nil {
		return nil, models.WatcherAuthResponse{}, fmt.Errorf("%s", err.Error())
	}

	err = ValidateMachine("test")
	if err != nil {
		log.Fatalln(err.Error())
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/watchers/login", strings.NewReader(body))
	req.Header.Add("User-Agent", UserAgent)
	router.ServeHTTP(w, req)

	loginResp := models.WatcherAuthResponse{}
	err = json.NewDecoder(w.Body).Decode(&loginResp)
	if err != nil {
		log.Fatalln(err.Error())
	}

	return router, loginResp, nil
}

func TestSimulatedAlert(t *testing.T) {
	router, loginResp, err := InitMachineTest()
	if err != nil {
		log.Fatalln(err.Error())
	}

	alertContentBytes, err := ioutil.ReadFile("./tests/alert_minibulk+simul.json")
	if err != nil {
		log.Fatal(err)
	}
	alertContent := string(alertContentBytes)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/alerts", strings.NewReader(alertContent))
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)

	//exclude decision in simulation mode
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?simulated=false", strings.NewReader(alertContent))
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), `"message":"Ip 91.121.79.178 performed crowdsecurity/ssh-bf (6 events over `)
	assert.NotContains(t, w.Body.String(), `"message":"Ip 91.121.79.179 performed crowdsecurity/ssh-bf (6 events over `)
	//include decision in simulation mode
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?simulated=true", strings.NewReader(alertContent))
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), `"message":"Ip 91.121.79.178 performed crowdsecurity/ssh-bf (6 events over `)
	assert.Contains(t, w.Body.String(), `"message":"Ip 91.121.79.179 performed crowdsecurity/ssh-bf (6 events over `)
}

func TestCreateAlert(t *testing.T) {
	router, loginResp, err := InitMachineTest()
	if err != nil {
		log.Fatalln(err.Error())
	}

	// Create Alert with invalid format
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/alerts", strings.NewReader("test"))
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)

	assert.Equal(t, 400, w.Code)
	assert.Equal(t, "{\"message\":\"invalid character 'e' in literal true (expecting 'r')\"}", w.Body.String())

	// Create Alert with invalid input
	alertContentBytes, err := ioutil.ReadFile("./tests/invalidAlert_sample.json")
	if err != nil {
		log.Fatal(err)
	}
	alertContent := string(alertContentBytes)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/v1/alerts", strings.NewReader(alertContent))
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)

	assert.Equal(t, 500, w.Code)
	assert.Equal(t, "{\"message\":\"validation failure list:\\nscenario in body is required\\nscenario_hash in body is required\\nscenario_version in body is required\\nsimulated in body is required\\nsource in body is required\"}", w.Body.String())

	// Create Valid Alert
	alertContentBytes, err = ioutil.ReadFile("./tests/alert_sample.json")
	if err != nil {
		log.Fatal(err)
	}
	alertContent = string(alertContentBytes)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/v1/alerts", strings.NewReader(alertContent))
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)

	assert.Equal(t, 201, w.Code)
	assert.Equal(t, "[\"1\"]", w.Body.String())
}

func TestAlertListFilters(t *testing.T) {
	router, loginResp, err := InitMachineTest()
	if err != nil {
		log.Fatalln(err.Error())
	}

	alertContentBytes, err := ioutil.ReadFile("./tests/alert_ssh-bf.json")
	if err != nil {
		log.Fatal(err)
	}

	alerts := make([]*models.Alert, 0)
	if err := json.Unmarshal(alertContentBytes, &alerts); err != nil {
		log.Fatal(err)
	}

	for _, alert := range alerts {
		*alert.StartAt = time.Now().Format(time.RFC3339)
		*alert.StopAt = time.Now().Format(time.RFC3339)
	}

	alertContent, err := json.Marshal(alerts)
	if err != nil {
		log.Fatal(err)
	}

	//create one alert
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/alerts", strings.NewReader(string(alertContent)))
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)

	//bad filter
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?test=test", strings.NewReader(string(alertContent)))
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 500, w.Code)
	assert.Equal(t, "{\"message\":\"Filter parameter 'test' is unknown (=test): invalid filter\"}", w.Body.String())

	//get without filters
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	//check alert and decision
	assert.Contains(t, w.Body.String(), "Ip 91.121.79.195 performed 'crowdsecurity/ssh-bf' (6 events over ")
	assert.Contains(t, w.Body.String(), `scope":"Ip","simulated":false,"start_ip":1534676931,"type":"ban","value":"91.121.79.195"`)

	//test decision_type filter (ok)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?decision_type=ban", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "Ip 91.121.79.195 performed 'crowdsecurity/ssh-bf' (6 events over ")
	assert.Contains(t, w.Body.String(), `scope":"Ip","simulated":false,"start_ip":1534676931,"type":"ban","value":"91.121.79.195"`)

	//test decision_type filter (bad value)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?decision_type=ratata", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "null", w.Body.String())

	//test scope (ok)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?scope=Ip", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "Ip 91.121.79.195 performed 'crowdsecurity/ssh-bf' (6 events over ")
	assert.Contains(t, w.Body.String(), `scope":"Ip","simulated":false,"start_ip":1534676931,"type":"ban","value":"91.121.79.195"`)

	//test scope (bad value)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?scope=rarara", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "null", w.Body.String())

	//test scenario (ok)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?scenario=crowdsecurity/ssh-bf", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "Ip 91.121.79.195 performed 'crowdsecurity/ssh-bf' (6 events over ")
	assert.Contains(t, w.Body.String(), `scope":"Ip","simulated":false,"start_ip":1534676931,"type":"ban","value":"91.121.79.195"`)

	//test scenario (bad value)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?scenario=crowdsecurity/nope", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "null", w.Body.String())

	//test ip (ok)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?ip=91.121.79.195", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "Ip 91.121.79.195 performed 'crowdsecurity/ssh-bf' (6 events over ")
	assert.Contains(t, w.Body.String(), `scope":"Ip","simulated":false,"start_ip":1534676931,"type":"ban","value":"91.121.79.195"`)

	//test ip (bad value)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?ip=99.122.77.195", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "null", w.Body.String())

	//test ip (invalid value)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?ip=gruueq", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 500, w.Code)
	assert.Equal(t, `{"message":"unable to parse 'gruueq': %!s(\u003cnil\u003e): invalid ip address / range"}`, w.Body.String())

	//test range (ok)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?range=91.121.79.0/24", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "Ip 91.121.79.195 performed 'crowdsecurity/ssh-bf' (6 events over ")
	assert.Contains(t, w.Body.String(), `scope":"Ip","simulated":false,"start_ip":1534676931,"type":"ban","value":"91.121.79.195"`)

	//test range
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?range=99.122.77.0/24", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "null", w.Body.String())

	//test range (invalid value)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?range=ratata", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 500, w.Code)
	assert.Equal(t, `{"message":"unable to convert 'ratata' to int interval: 'ratata' is not a valid CIDR: invalid ip address / range"}`, w.Body.String())

	//test since (ok)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?since=1h", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "Ip 91.121.79.195 performed 'crowdsecurity/ssh-bf' (6 events over ")
	assert.Contains(t, w.Body.String(), `scope":"Ip","simulated":false,"start_ip":1534676931,"type":"ban","value":"91.121.79.195"`)

	//test since (ok but yelds no results)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?since=1ns", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "null", w.Body.String())

	//test since (invalid value)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?since=1zuzu", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 500, w.Code)
	assert.Contains(t, w.Body.String(), `{"message":"while parsing duration: time: unknown unit`)

	//test until (ok)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?until=1ns", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "Ip 91.121.79.195 performed 'crowdsecurity/ssh-bf' (6 events over ")
	assert.Contains(t, w.Body.String(), `scope":"Ip","simulated":false,"start_ip":1534676931,"type":"ban","value":"91.121.79.195"`)

	//test until (ok but no return)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?until=1m", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "null", w.Body.String())

	//test until (invalid value)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?until=1zuzu", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 500, w.Code)
	assert.Contains(t, w.Body.String(), `{"message":"while parsing duration: time: unknown unit`)

	//test simulated (ok)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?simulated=true", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "Ip 91.121.79.195 performed 'crowdsecurity/ssh-bf' (6 events over ")
	assert.Contains(t, w.Body.String(), `scope":"Ip","simulated":false,"start_ip":1534676931,"type":"ban","value":"91.121.79.195"`)

	//test simulated (ok)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?simulated=false", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "Ip 91.121.79.195 performed 'crowdsecurity/ssh-bf' (6 events over ")
	assert.Contains(t, w.Body.String(), `scope":"Ip","simulated":false,"start_ip":1534676931,"type":"ban","value":"91.121.79.195"`)

	//test has active decision
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?has_active_decision=true", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "Ip 91.121.79.195 performed 'crowdsecurity/ssh-bf' (6 events over ")
	assert.Contains(t, w.Body.String(), `scope":"Ip","simulated":false,"start_ip":1534676931,"type":"ban","value":"91.121.79.195"`)

	//test has active decision
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?has_active_decision=false", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "null", w.Body.String())

	//test has active decision (invalid value)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?has_active_decision=ratatqata", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 500, w.Code)
	assert.Equal(t, `{"message":"'ratatqata' is not a boolean: strconv.ParseBool: parsing \"ratatqata\": invalid syntax: unable to parse type"}`, w.Body.String())

}

func TestAlertBulkInsert(t *testing.T) {
	router, loginResp, err := InitMachineTest()
	if err != nil {
		log.Fatalln(err.Error())
	}

	//insert a bulk of 20 alerts to trigger bulk insert
	alertContentBytes, err := ioutil.ReadFile("./tests/alert_bulk.json")
	if err != nil {
		log.Fatal(err)
	}
	alertContent := string(alertContentBytes)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/alerts", strings.NewReader(alertContent))
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts", strings.NewReader(alertContent))
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
}

func TestListAlert(t *testing.T) {
	router, loginResp, err := InitMachineTest()
	if err != nil {
		log.Fatalln(err.Error())
	}

	alertContentBytes, err := ioutil.ReadFile("./tests/alert_sample.json")
	if err != nil {
		log.Fatal(err)
	}
	alertContent := string(alertContentBytes)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/alerts", strings.NewReader(alertContent))
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)

	// List Alert with invalid filter
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts?test=test", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)
	assert.Equal(t, 500, w.Code)
	assert.Equal(t, "{\"message\":\"Filter parameter 'test' is unknown (=test): invalid filter\"}", w.Body.String())

	// List Alert
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/alerts", nil)
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "crowdsecurity/test")
}

func TestCreateAlertErrors(t *testing.T) {
	router, loginResp, err := InitMachineTest()
	if err != nil {
		log.Fatalln(err.Error())
	}

	alertContentBytes, err := ioutil.ReadFile("./tests/alert_sample.json")
	if err != nil {
		log.Fatal(err)
	}
	alertContent := string(alertContentBytes)

	//test invalid bearer
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/alerts", strings.NewReader(alertContent))
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", "ratata"))
	router.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)

	//test invalid bearer
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/v1/alerts", strings.NewReader(alertContent))
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token+"s"))
	router.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)

}

func TestDeleteAlert(t *testing.T) {
	router, loginResp, err := InitMachineTest()
	if err != nil {
		log.Fatalln(err.Error())
	}

	alertContentBytes, err := ioutil.ReadFile("./tests/alert_sample.json")
	if err != nil {
		log.Fatal(err)
	}
	alertContent := string(alertContentBytes)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/alerts", strings.NewReader(alertContent))
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	router.ServeHTTP(w, req)

	// Fail Delete Alert
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", "/v1/alerts", strings.NewReader(""))
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	req.RemoteAddr = "127.0.0.2:4242"
	router.ServeHTTP(w, req)

	assert.Equal(t, 403, w.Code)
	assert.Equal(t, `{"message":"access forbidden from this IP (127.0.0.2)"}`, w.Body.String())

	// Delete Alert
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", "/v1/alerts", strings.NewReader(""))
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", loginResp.Token))
	req.RemoteAddr = "127.0.0.1:4242"
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, `{"nbDeleted":"1"}`, w.Body.String())
}
