package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"
)

type pConfig struct {
	timeout int
	nodeIP string
	clusterName string
	webhookUrl string
	rescan int
	endpoints []string
}

type templateData struct {
	NodeIP string
	ClusterName string
	Comment string
	HostPort string
	Errmsg string
}

type User struct  {
	Id int 		 `json:"id"`
	Name string	 `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

var p pConfig

var gauge = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Namespace: "strongswan",
		Name:      "cassandratest",
		Help:      "1 is connected, 0 is disconnected",
	})

func init () {
	//setup logging outputs, required for k8s logging, then load the config and endpoints
	log.SetOutput(os.Stdout)
	log.SetOutput(os.Stderr)
	p = loadConfig()
}

func loadConfig () pConfig {
	r := pConfig{timeout: getEnvAsInt("TIMEOUT", 1),
		nodeIP:      getEnv("NODEIP", ""),
		clusterName: getEnv("CLUSTERNAME", ""),
		webhookUrl:  getEnv("WEBHOOKURL", ""),
		rescan:     getEnvAsInt("RESCAN", 30),
		endpoints: loadEPFile(getEnv("ENDPOINTS", "")),
	}
	return r
}

func parseJsonTemplate(a string,b templateData) string  {
	t, err := template.ParseFiles(a)
	if err != nil {
		panic(err)
	}
	buf := &bytes.Buffer{}
	err = t.Execute(buf, b)
	if err != nil {
		panic(err)
	}
	s := buf.String()
	return s
}

func sendTeamsNotification(webhookUrl string, payload string) error {
	resp, err := http.Post(webhookUrl, "application/json", bytes.NewBuffer([]byte(payload)))
	if err != nil {
		log.Fatal("Error sending Teams notification",err)
	}
	defer resp.Body.Close()
	return nil
}

// Simple helper function to read an environment or return a default value
func getEnv(key string, defaultVal string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}

	return defaultVal
}

// Simple helper function to read an environment variable into integer or return a default value
func getEnvAsInt(name string, defaultVal int) int {
	valueStr := getEnv(name, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}

	return defaultVal
}

func loadEPFile(epoints string) []string {
	var retSlice []string
	sliceData := strings.Split(string(epoints), "\n")
	for _, epdata := range sliceData {
		// ignore whitespace on empty lines and also comments
		if len(strings.TrimSpace(epdata)) != 0 && epdata[:1] != "#"{
			var firstWord = strings.Split(epdata," ")[0]
			retSlice = append(retSlice,firstWord)
		}
	}
	return retSlice
}

// test a connection to remote endpoint and port number, with a timeout
func testPort(endpoint string, tout int) (bool,string){
	timeOut := time.Duration(tout) * time.Second
	conn, err := net.DialTimeout("tcp", endpoint, timeOut)
	t := time.Now()
	if err != nil {
		errmsg := t.Format("2006-01-02-15:04:05") + fmt.Sprintf(" Node(%s) *Error*: No Connection to '%s' -- %s",p.nodeIP,endpoint,err )
		return true, errmsg
	}
	conn.Close()
	return false, "Success"
}

func jsonHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user := User{Id: 1,
		Name: "John Doe",
		Email: "johndoe@gmail.com",
		Phone: "000099999"}
	json.NewEncoder(w).Encode(user)
}


func checkEndpoints() {
	//TODO
	// add a retry element that doesnt alert until,the count reaches 2 minutes ?  i.e 120/p.rescan times
	// or let datadog/pagerduty handle that ?

	for _, ep := range p.endpoints {
		if len(ep) == 0 {
			continue
		} // ignore blank lines
		if strings.HasPrefix(ep, "#") {
			continue
		} // ignore yaml comments

		s := strings.Split(ep, " ")
		host := s[0]
		//comment := strings.Join(s[1:], " ")
		log.Print( "Checking ",host )
		//fmt.Printf("\tEndpoint(%s) Comment(%s) - ", host, comment)

		fail, errmsg := testPort(host, p.timeout)
		if fail {
			log.Printf("Failed %s\n", errmsg)
			gauge.Set(float64(0))
			//td := templateData{Comment: comment, ClusterName: p.clusterName,
			//	NodeIP: p.nodeIP, HostPort: host, Errmsg: errmsg}
			//payload := parseJsonTemplate("./templates/teams-alert.json", td)
			//
			//err := sendTeamsNotification(p.webhookUrl, payload)
			//if err != nil {
			//	log.Fatal("Couldn't send the Teams message: ", err)
			//}
		} else {
			gauge.Set(float64(1))
			log.Printf(" - Success\n")
		}
	}
}

func pollEndpoints() {
	for {
		//go checkEndpoints()
		log.Println("Polling")
		time.Sleep(time.Duration(p.rescan) * time.Second)

	}
}

func main() {
	log.Printf("Starting with a Rescan time (%d) amd a Port Timeout (%d)\n", p.rescan,p.timeout)
	prometheus.MustRegister(gauge)
	go pollEndpoints()  //check the endpoints every p.timeout seconds

	http.Handle("/metrics", promhttp.Handler())
	//http.HandleFunc("/json", jsonHandler)
	http.ListenAndServe(":8080", nil)
}